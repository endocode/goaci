package main

import (
	"exec"
	"flag"
	"os"
	"path/filepath"
	"strings"

	"github.com/appc/goaci/proj2aci"
)

type CmakePaths struct {
	src string
	build string
	install string
	binDir string
}

type CmakeOptions struct {
	binDir string
	reuseInstallDir string
}

type CmakeCustomization struct {
	paths       CmakePaths
	options     CmakeOptions
	fullBinPath string
}

func (custom *CmakeCustomization) Name() string {
	return "cmake"
}

func (custom *CmakeCustomization) GetPlaceholderMapping() map[string]string {
	return map[string]string{
		"<SRCPATH>":     custom.paths.src,
		"<BUILDPATH>":   custom.paths.build,
		"<INSTALLPATH>": custom.paths.install,
	}
}

func (custom *CmakeCustomization) SetupParameters(parameters *flag.FlagSet) {
	// --binary-dir
	flag.StringVar(&opts.binDir, "binary-dir", "", "Look for binaries in this directory (relative to install path, eg passing /usr/local/mysql/bin would look for a binary in /tmp/XXX/install/usr/local/mysql/bin")

	// --reuse-install-dir
	flag.StringVar(&opts.reuseInstallDir, "reuse-install-dir", "", "Instead of downloading a project, building and installing use this path with already installed project")
}

func (custom *CmakeCustomization) ValidateOptions(options *CommonOptions) error {
	if custom.options.reuseInstallDir != "" {
		fi, err := os.Stat(custom.options.reuseInstallDir)
		if err != nil || !fi.IsDir() {
			return fmt.Errorf("Error stating install dir to reuse")
		}
	}

	return nil
}

func (custom *CmakeCustomization) SetupPaths(paths *CommonPaths) error {
	custom.paths.src = filepath.Join(paths.tmpDir, "src")
	custom.paths.build = filepath.Join(paths.tmpDir, "build")
	if custom.options.reuseInstallDir != "" {
		custom.paths.install = custom.options.reuseInstallDir
	} else {
		custom.paths.install = filepath.Join(paths.tmpDir, "install")
	}
	return nil
}

func (custom *CmakeCustomization) GetDirectoriesToMake() []string {
	dirs := []string{
		custom.paths.src,
		custom.paths.build,
	}
	if custom.options.reuseInstallDir == "" {
		dirs = append(dirs, custom.paths.install)
	}
	return dirs
}

func (custom *CmakeCustomization) PrepareProject(options *CommonOptions, paths *CommonPaths) error {
	if opts.reuseInstallDir != "" {
		return nil
	}

	if err := custom.runShallowGitClone(options); err != nil {
		return err
	}

	if err := custom.runCmake(); err != nil {
		return err
	}

	if err := custom.runMake(); err != nil {
		return err
	}

	if err := custom.runMakeInstall(); err != nil {
		return err
	}

	if err := custom.findFullBinPath(options); err != nil {
		return err
	}

	return nil
}

// TODO: Replace with stuff from go tools
func (custom *CmakeCustomization) runShallowGitClone(options *CommonOptions) error {
	args := []string{
		"git",
		"clone",
		"--depth=1",
		fmt.Sprintf("https://%s", options.project),
		custom.paths.src,
	}
	return runCmd(args, nil, "")
}

func (custom *CmakeCustomization) runCmake() error {
	args := []string{
		"cmake",
		custom.paths.src,
	}
	return runCmd(args, nil, custom.paths.build)
}

func (custom *CmakeCustomization) runMake() error {
	args := []string{
		"make",
		fmt.Sprintf("-j%d", runtime.NumCPU()),
	}
	return runCmd(args, nil, custom.paths.build)
}

func (custom *CmakeCustomization) runMakeInstall() error {
	args := []string{
		"make",
		"install",
	}
	env := append(os.Environ(), "DESTDIR="+custom.paths.install)
	return runCmd(args, env, custom.paths.build)
}

func (custom *CmakeCustomization) findFullBinPath(options *CommonOptions) error {
	binDir, err := custom.getBinDir()
	if err != nil {
		return err
	}
	binary, err := proj2aci.GetBinaryName(binDir, options.useBinary)
	if err != nil {
		return err
	}
	custom.fullBinPath := filepath.Join(binDir, binary)
	return nil
}

func (custom *CmakeCustomization) getBinDir() (string, error) {
	if custom.options.binDir != "" {
		return filepath.Join(custom.paths.install, custom.options.binDir), nil
	}
	dirs := []string{
		"/usr/local/sbin",
		"/usr/local/bin",
		"/usr/sbin",
		"/usr/bin",
		"/sbin",
		"/bin",
	}
	for _, dir := range dirs {
		path := filepath.Join(custom.paths.install, dir)
		_, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		return path, nil
	}
	return "", fmt.Errorf("Could not find any bin directory")
}

func (custom *CmakeCustomization) GetAssets() ([]string, error) {
	assets, err := custom.createBinaryAssets()
	if err != nil {
		return err
	}
	opts.assets = append(opts.assets, assets...)
	return nil
}

func (custom *CmakeCustomization) createBinaryAssets() ([]string, error) {
	rootBinary := filepath.Join("/", filepath.Base(custom.fullBinPath))
	assets := []string{
		fmt.Sprintf("%s%s%s", rootBinary, ListSeparator(), fullBinPath),
	}
	buf := new(bytes.Buffer)
	args := []string{
		"ldd",
		binPath,
	}
	if err := runCmdFull(args, nil, "", buf, nil); err != nil {
		if _, ok := err.(CmdFailedError); !ok {
			return nil, err
		}
	} else {
		re := regexp.MustCompile(`(?m)^\t(?:\S+\s+=>\s+)?(\S+)\s+\([0-9a-fA-Fx]+\)$`)
		for _, matches := range re.FindAllStringSubmatch(string(buf.Bytes()), -1) {
			lib := matches[1]
			if lib == "" {
				continue
			}
			symlinkedAssets, err := getSymlinkedAssets(lib)
			if err != nil {
				return nil, err
			}
			assets = append(assets, symlinkedAssets...)
		}
	}

	return assets, nil
}

func getSymlinkedAssets(path string) ([]string, error) {
	assets := []string{}
	maxLevels := 100
	levels := maxLevels
	for {
		if levels < 1 {
			return nil, fmt.Errorf("Too many levels of symlinks (>$d)", maxLevels)
		}
		asset := fmt.Sprintf("%s%s%s", path, ListSeparator(), path)
		assets = append(assets, asset)
		fi, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				break
			}
			return nil, err
		}
		if fi.Mode()&os.ModeSymlink != os.ModeSymlink {
			break
		}
		symTarget, err := os.Readlink(path)
		if err != nil {
			return nil, err
		}
		if filepath.IsAbs(symTarget) {
			path = symTarget
		} else {
			path = filepath.Join(filepath.Dir(path), symTarget)
		}
		levels--
	}
	return assets, nil
}

func (custom *CmakeCustomization) GetImageFileName(options *CommonOptions) (string, error) {
	base := filepath.Base(options.project)
	if base == "..." {
		base = filepath.Base(filepath.Dir(options.project))
		if options.useBinary != "" {
			base += "-" + options.useBinary
		}
	}
	return base + schema.ACIExtension
}

func (custom *CmakeCustomization) GetImageACName(options *CommonOptions) (types.ACName, error) {
	imageACName := options.project
	if filepath.Base(imageACName) == "..." {
		imageACName = filepath.Dir(imageACName)
		if options.useBinary != "" {
			imageACName += "-" + options.useBinary
		}
	}
	return imageACName
}
