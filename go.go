package main

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type GoPaths struct {
	project string
	realGo string
	fakeGo string
	goRoot string
}

type GoOptions struct {
	goBinary string
	goPath string
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
		"<PROJPATH>": custom.projectPath,
		"<GOPATH>":   custom.goPath,
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
	flag.StringVar(&opts.option.goPath, "go-path", "", "Custom GOPATH (default: a temporary directory)")
}

func (custom *GoCustomization) ValidateOptions(options *CommonOptions) error {
	if custom.options.goBinary == "" {
		return fmt.Errorf("Go binary not found")
	}
	return nil
}

func (custom *GoCustomization) SetupPaths(paths *CommonPaths) error {
	custom.paths.realGo, custom.paths.fakeGo := custom.getGoPath(paths)

	if os.Getenv("GOPATH") != "" {
		Warn("GOPATH env var is ignored, use --go-path=\"$GOPATH\" option instead")
	}
	custom.paths.goRoot := os.Getenv("GOROOT")
	if goRoot != "" {
		Warn("Overriding GOROOT env var to ", goRoot)
	}

	// Project name is path-like string with slashes, but slash is
	// not a file separator on every OS.
	custom.paths.project := filepath.Join(goPath, "src", filepath.Join(strings.Split(projectName, "/")...))
	custom.paths.goBin := filepath.Join(fakeGoPath, "bin")
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

func (custom *GoCustomization) GetDirectoriesToMake() []string {
	return []string{
		custom.paths.fakeGo,
		custom.paths.goBin,
	}
}

func (custom *GoCustomization) PrepareProject(options *CommonOptions, paths *CommonPaths) error {
	// Construct args for a go get that does a static build
	args := []string{
		pathsNames.goExecPath,
		"get",
		"-a",
		"-tags", "netgo",
		"-ldflags", "'-w'",
		"-installsuffix", "nocgo", // for 1.4
		opts.project,
	}

	env := []string{
		"GOPATH=" + pathsNames.goPath,
		"GOBIN=" + pathsNames.goBinPath,
		"CGO_ENABLED=0",
		"PATH=" + os.Getenv("PATH"),
	}
	if pathsNames.goRootPath != "" {
		env = append(env, "GOROOT="+pathsNames.goRootPath)
	}

	cmd := exec.Cmd{
		Env:    env,
		Path:   pathsNames.goExecPath,
		Args:   args,
		Stderr: os.Stderr,
		Stdout: os.Stdout,
	}
	Debug("env: ", cmd.Env)
	Debug("running command: ", strings.Join(cmd.Args, " "))
	if err := cmd.Run(); err != nil {
		return err
	}
	err := custom.findBinaryName(); err != nil {
		return err
	}
	return nil
}

// getBinaryName get a binary name built by go get and selected by
// --use-binary parameter.
func (custom *GoCustomization) findBinaryName(options *CommonOptions, paths *CommonPaths) error {
	fi, err := ioutil.ReadDir(custom.paths.goBin)
	if err != nil {
		return err
	}

	switch {
	case len(fi) < 1:
		return fmt.Errorf("No binaries found in gobin.")
	case len(fi) == 1:
		name := fi[0].Name()
		if options.useBinary != "" && name != options.useBinary {
			return fmt.Errorf("No such binary found in gobin: %q. There is only %q", options.useBinary, name)
		}
		Debug("found binary: ", name)
		custom.app = name
		return nil
	case len(fi) > 1:
		names := []string{}
		for _, v := range fi {
			names = append(names, v.Name())
		}
		if options.useBinary == "" {
			return fmt.Errorf("Found multiple binaries in gobin, but --use-binary option is not used. Please specify which binary to put in ACI. Following binaries are available: %q", strings.Join(names, `", "`))
		}
		for _, v := range names {
			if v == options.useBinary {
				custom.app = v
				return nil
			}
		}
		return fmt.Errorf("No such binary found in gobin: %q. There are following binaries available: %q", options.useBinary, strings.Join(names, `", "`))
	}
	panic("Reaching this point shouldn't be possible.")
}

func (custom *GoCustomization) GetAssets() ([]string, error) {
	aciAsset := filepath.Join("/", custom.app)
	localAsset := filepath.Join(custom.paths.goBin, custom.app)
	asset := fmt.Sprintf("%s%s%s", aciAsset, FileSeparator(), localAsset)

	return []string{asset}, nil
}

func (custom *GoCustomization) GetImageFileName(options *CommonOptions) (string, error) {
	base := filepath.Base(options.project)
	if base == "..." {
		base = filepath.Base(filepath.Dir(options.project))
		if options.useBinary != "" {
			base += "-" + options.useBinary
		}
	}
	return base + schema.ACIExtension
}

func (custom *GoCustomization) GetImageACName(options *CommonOptions) (types.ACName, error) {
	imageACName := options.project
	if filepath.Base(imageACName) == "..." {
		imageACName = filepath.Dir(imageACName)
		if options.useBinary != "" {
			imageACName += "-" + options.useBinary
		}
	}
	return imageACName
}
