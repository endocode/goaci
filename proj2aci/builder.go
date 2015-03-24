package proj2aci

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	"github.com/appc/spec/aci"
	"github.com/appc/spec/schema"
	"github.com/appc/spec/schema/types"
)

type CommonConfiguration struct {
	Exec        []string
	UseBinary   string
	Assets      []string
	KeepTmpDir  bool
	TmpDir      string
	ReuseTmpDir string
	Project     string
}

type CommonPaths struct {
	TmpDir string
	AciDir string
	RootFS string
}

type BuilderCustomizations interface {
	Name() string
	GetCommonConfiguration() *CommonConfiguration
	ValidateConfiguration() error
	GetCommonPaths() *CommonPaths
	SetupPaths() error
	GetDirectoriesToMake() []string
	PrepareProject() error
	GetPlaceholderMapping() map[string]string
	GetAssets(aciBinDir string) ([]string, error)
	GetImageACName() (*types.ACName, error)
	GetBinaryName() string
	GetRepoPath() (string, error)
	GetImageFileName() (string, error)
}

type Builder struct {
	manifest      *schema.ImageManifest
	aciBinDir     string
	custom        BuilderCustomizations
}

func NewBuilder(custom BuilderCustomizations) *Builder {
	return &Builder{
		custom: custom,
		aciBinDir: "/",
	}
}

func (cmd *Builder) Name() string {
	return cmd.custom.Name()
}

func (cmd *Builder) Run() error {
	if err := cmd.validateConfiguration(); err != nil {
		return err
	}

	if err := cmd.setupPaths(); err != nil {
		return err
	}

	config := cmd.custom.GetCommonConfiguration()
	paths := cmd.custom.GetCommonPaths()
	if config.KeepTmpDir {
		Info(`Preserving temporary directory "`, paths.TmpDir, `"`)
	} else {
		defer os.RemoveAll(paths.TmpDir)
	}

	if config.ReuseTmpDir != "" {
		if err := os.RemoveAll(paths.AciDir); err != nil {
			return err
		}

		if err := cmd.makeDirectories(); err != nil {
			return err
		}
	} else {
		if err := cmd.makeDirectories(); err != nil {
			return err
		}

		if err := cmd.prepareProject(); err != nil {
			return err
		}
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

func (cmd *Builder) validateConfiguration() error {
	config := cmd.custom.GetCommonConfiguration()
	if config == nil {
		panic("common configuration is nil")
	}
	if config.Project == "" {
		fmt.Errorf("Got no project to build")
	}

	if config.TmpDir != "" && config.ReuseTmpDir != "" && config.TmpDir != config.ReuseTmpDir {
		return fmt.Errorf("Specified both tmp dir to reuse and a tmp dir and they are different. ")
	}
	if !DirExists(config.ReuseTmpDir) {
		return fmt.Errorf("Invalid tmp dir to reuse")
	}

	return cmd.custom.ValidateConfiguration()
}

func (cmd *Builder) setupPaths() error {
	config := cmd.custom.GetCommonConfiguration()
	paths := cmd.custom.GetCommonPaths()
	tmpDir := ""
	if config.TmpDir != "" {
		tmpDir = config.TmpDir
	} else if config.ReuseTmpDir != "" {
		tmpDir = config.ReuseTmpDir
	} else {
		tmpName := fmt.Sprintf("proj2aci-%s", cmd.custom.Name())
		aTmpDir, err := ioutil.TempDir("", tmpName)
		if err != nil {
			return fmt.Errorf("Failed to set up temporary directory: %v", err)
		}
		tmpDir = aTmpDir
	}
	paths.TmpDir = tmpDir
	paths.AciDir = filepath.Join(paths.TmpDir, "aci")
	paths.RootFS = filepath.Join(paths.AciDir, "rootfs")
	return cmd.custom.SetupPaths()
}

func (cmd *Builder) makeDirectories() error {
	paths := cmd.custom.GetCommonPaths()
	config := cmd.custom.GetCommonConfiguration()

	toMake := []string{
		paths.AciDir,
		paths.RootFS,
	}
	if config.ReuseTmpDir == "" {
		toMake = append(toMake, cmd.custom.GetDirectoriesToMake()...)
		if config.TmpDir != "" {
			toMake = append(toMake, paths.TmpDir)
		}
	}

	for _, dir := range toMake {
		if err := os.Mkdir(dir, 0755); err != nil {
			return fmt.Errorf("Failed to make directory %q: %v", dir, err)
		}
	}
	return nil
}

func (cmd *Builder) prepareProject() error {
	return cmd.custom.PrepareProject()
}

func (cmd *Builder) copyAssets() error {
	paths := cmd.custom.GetCommonPaths()
	config := cmd.custom.GetCommonConfiguration()
	mapping := cmd.custom.GetPlaceholderMapping()
	customAssets, err := cmd.custom.GetAssets(cmd.aciBinDir)
	if err != nil {
		return err
	}
	assets := append(config.Assets, customAssets...)
	if err := PrepareAssets(assets, paths.RootFS, mapping); err != nil {
		return err
	}
	return nil
}

func (cmd *Builder) prepareManifest() error {
	name, err := cmd.custom.GetImageACName()
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

func (cmd *Builder) getApp() *types.App {
	exec := []string{filepath.Join("/", cmd.custom.GetBinaryName())}
	config := cmd.custom.GetCommonConfiguration()

	return &types.App{
		Exec:  append(exec, config.Exec...),
		User:  "0",
		Group: "0",
	}
}

func (cmd *Builder) getLabels() (types.Labels, error) {
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

func (cmd *Builder) getVCSLabel() (*types.Label, error) {
	repoPath, err := cmd.custom.GetRepoPath()
	if err != nil {
		return nil, err
	}
	if repoPath == "" {
		return nil, nil
	}
	name, value, err := GetVCSInfo(repoPath)
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

func (cmd *Builder) writeACI() error {
	mode := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	filename, err := cmd.custom.GetImageFileName()
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
	paths := cmd.custom.GetCommonPaths()
	if err := filepath.Walk(paths.AciDir, aci.BuildWalker(paths.AciDir, iw)); err != nil {
		return err
	}
	if err := iw.Close(); err != nil {
		return err
	}
	Info("Wrote ", of.Name())
	return nil
}
