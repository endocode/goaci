package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/appc/spec/schema"
	"github.com/appc/spec/schema/types"
	"github.com/appc/goaci/proj2aci"
)

type GoPaths struct {
	project string
	realGo  string
	fakeGo  string
	goRoot  string
	goBin   string
}

type GoOptions struct {
	goBinary string
	goPath   string
}

type GoCustomization struct {
	paths   GoPaths
	options GoOptions
	app     string
}

func (custom *GoCustomization) Name() string {
	return "go"
}

func (custom *GoCustomization) GetPlaceholderMapping() map[string]string {
	return map[string]string{
		"<PROJPATH>": custom.paths.project,
		"<GOPATH>":   custom.paths.realGo,
	}
}

func (custom *GoCustomization) SetupParameters(parameters *flag.FlagSet) {
	// --go-binary
	goDefaultBinaryDesc := "Go binary to use"
	gocmd, err := exec.LookPath("go")
	if err != nil {
		goDefaultBinaryDesc += " (default: none found in $PATH, so it must be provided)"
	} else {
		goDefaultBinaryDesc += " (default: whatever go in $PATH)"
	}
	flag.StringVar(&custom.options.goBinary, "go-binary", gocmd, goDefaultBinaryDesc)

	// --go-path
	flag.StringVar(&custom.options.goPath, "go-path", "", "Custom GOPATH (default: a temporary directory)")
}

func (custom *GoCustomization) ValidateOptions(options *CommonOptions) error {
	if custom.options.goBinary == "" {
		return fmt.Errorf("Go binary not found")
	}
	return nil
}

func (custom *GoCustomization) SetupPaths(paths *CommonPaths, project string) error {
	custom.paths.realGo, custom.paths.fakeGo = custom.getGoPath(paths)

	if os.Getenv("GOPATH") != "" {
		proj2aci.Warn("GOPATH env var is ignored, use --go-path=\"$GOPATH\" option instead")
	}
	custom.paths.goRoot = os.Getenv("GOROOT")
	if custom.paths.goRoot != "" {
		proj2aci.Warn("Overriding GOROOT env var to ", custom.paths.goRoot)
	}

	projectName := getProjectName(project)
	// Project name is path-like string with slashes, but slash is
	// not a file separator on every OS.
	custom.paths.project = filepath.Join(custom.paths.realGo, "src", filepath.Join(strings.Split(projectName, "/")...))
	custom.paths.goBin = filepath.Join(custom.paths.fakeGo, "bin")
	return nil
}

// getGoPath returns go path and fake go path. The former is either in
// /tmp (which is a default) or some other path as specified by
// --go-path parameter. The latter is always in /tmp.
func (custom *GoCustomization) getGoPath(paths *CommonPaths) (string, string) {
	fakeGoPath := filepath.Join(paths.tmpDir, "gopath")
	if custom.options.goPath == "" {
		return fakeGoPath, fakeGoPath
	}
	return custom.options.goPath, fakeGoPath
}

func getProjectName(project string) string {
	if filepath.Base(project) != "..." {
		return project
	}
	return filepath.Dir(project)
}

func (custom *GoCustomization) GetDirectoriesToMake() []string {
	return []string{
		custom.paths.fakeGo,
		custom.paths.goBin,
	}
}

func (custom *GoCustomization) PrepareProject(options *CommonOptions, paths *CommonPaths) error {
	// Construct args for a go get that does a static build
	args := []string{
		"go",
		"get",
		"-a",
		"-tags", "netgo",
		"-ldflags", "'-w'",
		"-installsuffix", "nocgo", // for 1.4
		options.project,
	}

	env := []string{
		"GOPATH=" + custom.paths.realGo,
		"GOBIN=" + custom.paths.goBin,
		"CGO_ENABLED=0",
		"PATH=" + os.Getenv("PATH"),
	}
	if custom.paths.goRoot != "" {
		env = append(env, "GOROOT="+custom.paths.goRoot)
	}

	cmd := exec.Cmd{
		Env:    env,
		Path:   custom.options.goBinary,
		Args:   args,
		Stderr: os.Stderr,
		Stdout: os.Stdout,
	}
	proj2aci.Debug("env: ", cmd.Env)
	proj2aci.Debug("running command: ", strings.Join(cmd.Args, " "))
	if err := cmd.Run(); err != nil {
		return err
	}
	if err := custom.findBinaryName(options); err != nil {
		return err
	}
	return nil
}

// getBinaryName get a binary name built by go get and selected by
// --use-binary parameter.
func (custom *GoCustomization) findBinaryName(options *CommonOptions) error {
	binaryName, err := proj2aci.GetBinaryName(custom.paths.goBin, options.useBinary)
	if err != nil {
		return err
	}
	custom.app = binaryName
	return nil
}

func (custom *GoCustomization) GetAssets(aciBinDir string) ([]string, error) {
	aciAsset := filepath.Join(aciBinDir, custom.app)
	localAsset := filepath.Join(custom.paths.goBin, custom.app)

	return []string{proj2aci.GetAssetString(aciAsset, localAsset)}, nil
}

func (custom *GoCustomization) GetImageACName(options *CommonOptions) (*types.ACName, error) {
	imageACName := options.project
	if filepath.Base(imageACName) == "..." {
		imageACName = filepath.Dir(imageACName)
		if options.useBinary != "" {
			imageACName += "-" + options.useBinary
		}
	}
	return types.NewACName(imageACName)
}

func (custom *GoCustomization) GetBinaryName() string {
	return custom.app
}

func (custom *GoCustomization) GetRepoPath() (string, error) {
	return custom.paths.project, nil
}

func (custom *GoCustomization) GetImageFileName(options *CommonOptions) (string, error) {
	base := filepath.Base(options.project)
	if base == "..." {
		base = filepath.Base(filepath.Dir(options.project))
		if options.useBinary != "" {
			base += "-" + options.useBinary
		}
	}
	return base + schema.ACIExtension, nil
}
