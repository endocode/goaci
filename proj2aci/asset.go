package proj2aci

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func GetAssetString(aciAsset, localAsset string) string {
	return getAssetString(aciAsset, localAsset)
}

func PrepareAssets(assets []string, rootfs string, placeholderMapping map[string]string) error {
	newAssets := assets
	for len(newAssets) > 0 {
		assetsToProcess := newAssets
		newAssets = nil
		for _, asset := range assetsToProcess {
			additionalAssets, err := processAsset(asset, rootfs, placeholderMapping)
			if err != nil {
				return err
			}
			newAssets = append(newAssets, additionalAssets...)
		}
	}
	return nil
}

func processAsset(asset, rootfs string, placeholderMapping map[string]string) ([]string, error) {
	splitAsset := filepath.SplitList(asset)
	if len(splitAsset) != 2 {
		return nil, fmt.Errorf("Malformed asset option: '%v' - expected two absolute paths separated with %v", asset, ListSeparator())
	}
	ACIAsset := replacePlaceholders(splitAsset[0], placeholderMapping)
	localAsset := replacePlaceholders(splitAsset[1], placeholderMapping)
	if err := validateAsset(ACIAsset, localAsset); err != nil {
		return nil, err
	}
	ACIAssetSubPath := filepath.Join(rootfs, filepath.Dir(ACIAsset))
	err := os.MkdirAll(ACIAssetSubPath, 0755)
	if err != nil {
		return nil, fmt.Errorf("Failed to create directory tree for asset '%v': %v", asset, err)
	}
	err = copyTree(localAsset, filepath.Join(rootfs, ACIAsset))
	if err != nil {
		return nil, fmt.Errorf("Failed to copy assets for %q: %v", asset, err)
	}
	additionalAssets, err := getSoLibs(localAsset)
	if err != nil {
		return nil, fmt.Errorf("Failed to get dependent assets for %q: %v", localAsset, err)
	}
	return additionalAssets, nil
}

func replacePlaceholders(path string, placeholderMapping map[string]string) string {
	Debug("Processing path: ", path)
	newPath := path
	for placeholder, replacement := range placeholderMapping {
		newPath = strings.Replace(newPath, placeholder, replacement, -1)
	}
	Debug("Processed path: ", newPath)
	return newPath
}

func validateAsset(ACIAsset, localAsset string) error {
	if !filepath.IsAbs(ACIAsset) {
		return fmt.Errorf("Wrong ACI asset: '%v' - ACI asset has to be absolute path", ACIAsset)
	}
	if !filepath.IsAbs(localAsset) {
		return fmt.Errorf("Wrong local asset: '%v' - local asset has to be absolute path", localAsset)
	}
	fi, err := os.Stat(localAsset)
	if err != nil {
		return fmt.Errorf("Error stating %v: %v", localAsset, err)
	}
	mode := fi.Mode()
	if mode.IsDir() || mode.IsRegular() || isSymlink(mode) {
		return nil
	}
	return fmt.Errorf("Can't handle local asset %v - not a file, not a dir, not a symlink", fi.Name())
}

func copyTree(src, dest string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rootLess := path[len(src):]
		target := filepath.Join(dest, rootLess)
		mode := info.Mode()
		switch {
		case mode.IsDir():
			err := os.Mkdir(target, mode.Perm())
			if err != nil {
				return err
			}
		case mode.IsRegular():
			if err := copyRegularFile(path, target); err != nil {
				return err
			}
		case isSymlink(mode):
			if err := copySymlink(path, target); err != nil {
				return err
			}
		default:
			return fmt.Errorf("Unsupported node %q in assets, only regular files, directories and symlinks are supported.", path, mode.String())
		}
		return nil
	})
}

func copyRegularFile(src, dest string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destFile.Close()
	if _, err := io.Copy(destFile, srcFile); err != nil {
		return err
	}
	fi, err := srcFile.Stat()
	if err != nil {
		return err
	}
	if err := destFile.Chmod(fi.Mode().Perm()); err != nil {
		return err
	}
	return nil
}

func copySymlink(src, dest string) error {
	symTarget, err := os.Readlink(src)
	if err != nil {
		return err
	}
	if err := os.Symlink(symTarget, dest); err != nil {
		return err
	}
	return nil
}

func getSoLibs(path string) ([]string, error) {
	assets := []string{}
	buf := new(bytes.Buffer)
	args := []string{
		"ldd",
		path,
	}
	if err := RunCmdFull("", args, nil, "", buf, nil); err != nil {
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
		asset := getAssetString(path, path)
		assets = append(assets, asset)
		fi, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				break
			}
			return nil, err
		}
		if !isSymlink(fi.Mode()) {
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

func getAssetString(aciAsset, localAsset string) string {
	return fmt.Sprintf("%s%s%s", aciAsset, ListSeparator(), localAsset)
}

func isSymlink(mode os.FileMode) bool {
	return mode&os.ModeSymlink == os.ModeSymlink
}
