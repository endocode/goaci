package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/appc/spec/schema"
	"github.com/appc/spec/schema/types"
	"github.com/appc/goaci/proj2aci"
)

type CmakePaths struct {
	src     string
	build   string
	install string
	binDir  string
}

type CmakeOptions struct {
	binDir          string
	reuseInstallDir string
	reuseSrcDir     string
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
	flag.StringVar(&custom.options.binDir, "binary-dir", "", "Look for binaries in this directory (relative to install path, eg passing /usr/local/mysql/bin would look for a binary in /tmp/XXX/install/usr/local/mysql/bin")

	// --reuse-install-dir
	flag.StringVar(&custom.options.reuseInstallDir, "reuse-install-dir", "", "Instead of downloading, building and installing a project, use this path with already installed project")

	// --reuse-src-dir
	flag.StringVar(&custom.options.reuseSrcDir, "reuse-src-dir", "", "Instead of downloading a project, use this path with already downloaded sources")
}

func (custom *CmakeCustomization) ValidateOptions(options *CommonOptions) error {
	if !dirExists(custom.options.reuseInstallDir) {
		return fmt.Errorf("Invalid install dir to reuse")
	}
	if !dirExists(custom.options.reuseSrcDir) {
		return fmt.Errorf("Invalid src dir to reuse")
	}

	return nil
}

func dirExists(path string) bool {
	if path != "" {
		fi, err := os.Stat(path)
		if err != nil || !fi.IsDir() {
			return false
		}
	}
	return true
}

func (custom *CmakeCustomization) SetupPaths(paths *CommonPaths, project string) error {
	setupReusableDir(&custom.paths.src, custom.options.reuseInstallDir, filepath.Join(paths.tmpDir, "src"))
	custom.paths.build = filepath.Join(paths.tmpDir, "build")
	setupReusableDir(&custom.paths.install, custom.options.reuseInstallDir, filepath.Join(paths.tmpDir, "install"))
	return nil
}

func setupReusableDir(path *string, reusePath, stockPath string) {
	if path == nil {
		panic("path in setupReusableDir cannot be nil")
	}
	if reusePath != "" {
		*path = reusePath
	} else {
		*path = stockPath
	}
}

func (custom *CmakeCustomization) GetDirectoriesToMake() []string {
	dirs := []string{
		custom.paths.build,
	}
	if custom.options.reuseInstallDir == "" {
		dirs = append(dirs, custom.paths.install)
	}
	if custom.options.reuseSrcDir == "" {
		dirs = append(dirs, custom.paths.src)
	}
	return dirs
}

func (custom *CmakeCustomization) PrepareProject(options *CommonOptions, paths *CommonPaths) error {
	if custom.options.reuseInstallDir != "" {
		return nil
	}

	if custom.options.reuseSrcDir == "" {
		if err := custom.runShallowGitClone(options); err != nil {
			return err
		}
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
	return proj2aci.RunCmd(args, nil, "")
}

func (custom *CmakeCustomization) runCmake() error {
	args := []string{
		"cmake",
		custom.paths.src,
	}
	return proj2aci.RunCmd(args, nil, custom.paths.build)
}

func (custom *CmakeCustomization) runMake() error {
	args := []string{
		"make",
		fmt.Sprintf("-j%d", runtime.NumCPU()),
	}
	return proj2aci.RunCmd(args, nil, custom.paths.build)
}

func (custom *CmakeCustomization) runMakeInstall() error {
	args := []string{
		"make",
		"install",
	}
	env := append(os.Environ(), "DESTDIR="+custom.paths.install)
	return proj2aci.RunCmd(args, env, custom.paths.build)
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
	custom.fullBinPath = filepath.Join(binDir, binary)
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

func (custom *CmakeCustomization) GetAssets(aciBinDir string) ([]string, error) {
	return custom.createBinaryAsset(aciBinDir)
}

func (custom *CmakeCustomization) createBinaryAsset(aciBinDir string) ([]string, error) {
	rootBinary := filepath.Join(aciBinDir, filepath.Base(custom.fullBinPath))
	return []string{proj2aci.GetAssetString(rootBinary, custom.fullBinPath)}, nil
}

func (custom *CmakeCustomization) GetImageACName(options *CommonOptions) (*types.ACName, error) {
	imageACName := options.project
	if filepath.Base(imageACName) == "..." {
		imageACName = filepath.Dir(imageACName)
		if options.useBinary != "" {
			imageACName += "-" + options.useBinary
		}
	}
	return types.NewACName(imageACName)
}

func (custom *CmakeCustomization) GetBinaryName() string {
	return filepath.Base(custom.fullBinPath)
}

func (custom *CmakeCustomization) GetRepoPath() (string, error) {
	return custom.paths.src, nil
}

func (custom *CmakeCustomization) GetImageFileName(options *CommonOptions) (string, error) {
	base := filepath.Base(options.project)
	if base == "..." {
		base = filepath.Base(filepath.Dir(options.project))
		if options.useBinary != "" {
			base += "-" + options.useBinary
		}
	}
	return base + schema.ACIExtension, nil
}
