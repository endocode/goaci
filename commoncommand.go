package main

import (
	"archive/tar"
	"compress/gzip"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sort"

	"github.com/appc/spec/aci"
	"github.com/appc/spec/schema"
	"github.com/appc/spec/schema/types"
	"github.com/appc/goaci/proj2aci"
)

type StringVector []string

func (v *StringVector) String() string {
	return `"` + strings.Join(*v, `" "`) + `"`
}

func (v *StringVector) Set(str string) error {
	*v = append(*v, str)
	return nil
}

type CommonOptions struct {
	exec      StringVector
	useBinary string
	assets    StringVector
	keepTmp   bool
	project   string
}

type CommonPaths struct {
	tmpDir string
	aciDir string
	rootFS string
}

type Customizations interface {
	Name() string
	GetPlaceholderMapping() map[string]string
	SetupParameters(parameters *flag.FlagSet)
	ValidateOptions(options *CommonOptions) error
	SetupPaths(paths *CommonPaths, project string) error
	GetDirectoriesToMake() []string
	PrepareProject(options *CommonOptions, paths *CommonPaths) error
	GetAssets(aciBinDir string) ([]string, error)
	GetImageACName(options *CommonOptions) (*types.ACName, error)
	GetBinaryName() string
	GetRepoPath() (string, error)
	GetImageFileName(options *CommonOptions) (string, error)
}

type CommonCommand struct {
	options   CommonOptions
	paths     CommonPaths
	manifest  *schema.ImageManifest
	aciBinDir string
	custom    Customizations
}

func NewCommonCommand(custom Customizations) Command {
	return &CommonCommand{
		custom: custom,
		aciBinDir: "/",
	}
}

func (cmd *CommonCommand) Name() string {
	return cmd.custom.Name()
}

func (cmd *CommonCommand) Run(args []string) error {
	if err := cmd.setupOptions(args); err != nil {
		return err
	}

	if err := cmd.setupPaths(); err != nil {
		return err
	}

	if cmd.options.keepTmp {
		proj2aci.Info(`Preserving temporary directory "`, cmd.paths.tmpDir, `"`)
	} else {
		defer os.RemoveAll(cmd.paths.tmpDir)
	}

	if err := cmd.makeDirectories(); err != nil {
		return err
	}

	if err := cmd.prepareProject(); err != nil {
		return err
	}

	if err := cmd.copyAssets(); err != nil {
		return err
	}

	if err := cmd.prepareManifest(); err != nil {
		return err
	}

	if err := cmd.writeACI(); err != nil {
		return err
	}

	return nil
}

func (cmd *CommonCommand) setupOptions(args []string) error {
	parameters := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	cmd.setupParameters(parameters)
	if err := parameters.Parse(args); err != nil {
		return err
	}
	if err := cmd.validateOptions(parameters); err != nil {
		return err
	}
	return nil
}

func (cmd *CommonCommand) setupParameters(parameters *flag.FlagSet) {
	// --exec
	parameters.Var(&cmd.options.exec, "exec", "Parameters passed to app, can be used multiple times")

	// --use-binary
	parameters.StringVar(&cmd.options.useBinary, "use-binary", "", "Which executable to put in ACI image")

	// --asset
	mapping := cmd.custom.GetPlaceholderMapping();
	placeholders := make([]string, 0, len(mapping))
	for p, _ := range mapping {
		placeholders = append(placeholders, p)
	}
	sort.Strings(placeholders)
	parameters.Var(&cmd.options.assets, "asset", "Additional assets, can be used multiple times; format: "+proj2aci.GetAssetString("<path in ACI rootfs>","<local path>")+"; available placeholders for use: " + strings.Join(placeholders, ", "))

	// --keep-tmp
	parameters.BoolVar(&cmd.options.keepTmp, "keep-tmp", false, "Do not delete temporary directory used for creating ACI")

	cmd.custom.SetupParameters(parameters)
}

func (cmd *CommonCommand) validateOptions(parameters *flag.FlagSet) error {
	args := flag.Args()
	if len(args) != 1 {
		return fmt.Errorf("Expected exactly one project to build, got %d", len(args))
	}
	cmd.options.project = args[0]
	return cmd.custom.ValidateOptions(&cmd.options)
}

func (cmd *CommonCommand) setupPaths() error {
	tmpName := fmt.Sprintf("proj2aci-%s", cmd.custom.Name())
	tmpDir, err := ioutil.TempDir("", tmpName)
	if err != nil {
		return fmt.Errorf("Failed to set up temporary directory: %v", err)
	}
	cmd.paths.tmpDir = tmpDir
	cmd.paths.aciDir = filepath.Join(cmd.paths.tmpDir, "aci")
	cmd.paths.rootFS = filepath.Join(cmd.paths.aciDir, "rootfs")
	return cmd.custom.SetupPaths(&cmd.paths, cmd.options.project)
}

func (cmd *CommonCommand) makeDirectories() error {
	toMake := []string{
		cmd.paths.aciDir,
		cmd.paths.rootFS,
	}
	toMake = append(toMake, cmd.custom.GetDirectoriesToMake()...)

	for _, dir := range toMake {
		if err := os.Mkdir(dir, 0755); err != nil {
			return fmt.Errorf("Failed to make directory %q: %v", dir, err)
		}
	}
	return nil
}

func (cmd *CommonCommand) prepareProject() error {
	return cmd.custom.PrepareProject(&cmd.options, &cmd.paths)
}

func (cmd *CommonCommand) copyAssets() error {
	mapping := cmd.custom.GetPlaceholderMapping()
	customAssets, err := cmd.custom.GetAssets(cmd.aciBinDir)
	if err != nil {
		return err
	}
	assets := append(cmd.options.assets, customAssets...)
	if err := proj2aci.PrepareAssets(assets, cmd.paths.rootFS, mapping); err != nil {
		return err
	}
	return nil
}

func (cmd *CommonCommand) prepareManifest() error {
	name, err := cmd.custom.GetImageACName(&cmd.options)
	if err != nil {
		return err
	}
	labels, err := cmd.getLabels()
	if err != nil {
		return err
	}

	cmd.manifest = schema.BlankImageManifest()
	cmd.manifest.Name = *name
	cmd.manifest.App = cmd.getApp()
	cmd.manifest.Labels = labels
	return nil
}

func (cmd *CommonCommand) getApp() *types.App {
	exec := []string{filepath.Join("/", cmd.custom.GetBinaryName())}

	return &types.App{
		Exec:  append(exec, cmd.options.exec...),
		User:  "0",
		Group: "0",
	}
}

func (cmd *CommonCommand) getLabels() (types.Labels, error) {
	arch, err := newLabel("arch", runtime.GOARCH)
	if err != nil {
		return nil, err
	}
	os, err := newLabel("os", runtime.GOOS)
	if err != nil {
		return nil, err
	}

	labels := types.Labels{
		*arch,
		*os,
	}

	vcsLabel, err := cmd.getVCSLabel()
	if err != nil {
		return nil, err
	} else if vcsLabel != nil {
		labels = append(labels, *vcsLabel)
	}

	return labels, nil
}

func newLabel(name, value string) (*types.Label, error) {
	acName, err := types.NewACName(name)
	if err != nil {
		return nil, err
	}
	return &types.Label{
		Name: *acName,
		Value: value,
	}, nil
}

func (cmd *CommonCommand) getVCSLabel() (*types.Label, error) {
	repoPath, err := cmd.custom.GetRepoPath()
	if err != nil {
		return nil, err
	}
	if repoPath == "" {
		return nil, nil
	}
	name, value, err := proj2aci.GetVCSInfo(repoPath)
	if err != nil {
		return nil, fmt.Errorf("Failed to get VCS info: %v", err)
	}
	acname, err := types.NewACName(name)
	if err != nil {
		return nil, fmt.Errorf("Invalid VCS label: %v", err)
	}
	return &types.Label{
		Name:  *acname,
		Value: value,
	}, nil
}

func (cmd *CommonCommand) writeACI() error {
	mode := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	filename, err := cmd.custom.GetImageFileName(&cmd.options)
	if err != nil {
		return err
	}
	of, err := os.OpenFile(filename, mode, 0644)
	if err != nil {
		return fmt.Errorf("Error opening output file: %v", err)
	}
	defer of.Close()

	gw := gzip.NewWriter(of)
	defer gw.Close()

	tr := tar.NewWriter(gw)
	defer tr.Close()

	// FIXME: the files in the tar archive are added with the
	// wrong uid/gid. The uid/gid of the aci builder leaks in the
	// tar archive. See: https://github.com/appc/goaci/issues/16
	iw := aci.NewImageWriter(*cmd.manifest, tr)
	if err := filepath.Walk(cmd.paths.aciDir, aci.BuildWalker(cmd.paths.aciDir, iw)); err != nil {
		return err
	}
	if err := iw.Close(); err != nil {
		return err
	}
	proj2aci.Info("Wrote ", of.Name())
	return nil
}
