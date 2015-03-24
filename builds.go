package main

import (
	"flag"
	"os/exec"
	"sort"
	"strings"

	"github.com/appc/goaci/proj2aci"
)

type stringVectorWrapper struct {
	vector *[]string
}

func (wrapper *stringVectorWrapper) String() string {
	if len(*wrapper.vector) > 0 {
		return `["` + strings.Join(*wrapper.vector, `" "`) + `"]`
	}
	return "[]"
}

func (wrapper *stringVectorWrapper) Set(str string) error {
	*wrapper.vector = append(*wrapper.vector, str)
	return nil
}

type commonBuild struct {
	custom       proj2aci.BuilderCustomizations
	config       *proj2aci.CommonConfiguration
	execWrapper  stringVectorWrapper
	assetWrapper stringVectorWrapper
}

func (build *commonBuild) setupCommonParameters(parameters *flag.FlagSet) {
	// --exec
	build.execWrapper.vector = &build.config.Exec
	parameters.Var(&build.execWrapper, "exec", "Parameters passed to app, can be used multiple times")

	// --use-binary
	parameters.StringVar(&build.config.UseBinary, "use-binary", "", "Which executable to put in ACI image")

	// --asset
	build.assetWrapper.vector = &build.config.Assets
	parameters.Var(&build.assetWrapper, "asset", "Additional assets, can be used multiple times; format: "+proj2aci.GetAssetString("<path in ACI rootfs>", "<local path>")+"; available placeholders for use: "+build.getPlaceholders())

	// --keep-tmp-dir
	parameters.BoolVar(&build.config.KeepTmpDir, "keep-tmp-dir", false, "Do not delete temporary directory used for creating ACI")

	// --tmp-dir
	parameters.StringVar(&build.config.TmpDir, "tmp-dir", "", "Use this directory for build a project and an ACI image")

	// --reuse-tmp-dir
	parameters.StringVar(&build.config.ReuseTmpDir, "reuse-tmp-dir", "", "Use this already existing directory with built project to build an ACI image; ACI specific contents in this directory are removed before reuse")
}

func (build *commonBuild) getPlaceholders() string {
	mapping := build.custom.GetPlaceholderMapping()
	placeholders := make([]string, 0, len(mapping))
	for p := range mapping {
		placeholders = append(placeholders, p)
	}
	sort.Strings(placeholders)
	return strings.Join(placeholders, ", ")
}

func (build *commonBuild) Name() string {
	return build.custom.Name()
}

func (build *commonBuild) GetBuilderCustomizations() proj2aci.BuilderCustomizations {
	return build.custom
}

type goBuild struct {
	commonBuild

	goCustom *proj2aci.GoCustomizations
}

func newGoBuild() build {
	custom := &proj2aci.GoCustomizations{}
	return &goBuild{
		commonBuild: commonBuild{
			custom: custom,
			config: custom.GetCommonConfiguration(),
		},
		goCustom: custom,
	}
}

func (build *goBuild) SetupParameters(parameters *flag.FlagSet) {
	// common params
	build.setupCommonParameters(parameters)

	// --go-binary
	goDefaultBinaryDesc := "Go binary to use"
	gocmd, err := exec.LookPath("go")
	if err != nil {
		goDefaultBinaryDesc += " (default: none found in $PATH, so it must be provided)"
	} else {
		goDefaultBinaryDesc += " (default: whatever go in $PATH)"
	}
	parameters.StringVar(&build.goCustom.Configuration.GoBinary, "go-binary", gocmd, goDefaultBinaryDesc)

	// --go-path
	parameters.StringVar(&build.goCustom.Configuration.GoPath, "go-path", "", "Custom GOPATH (default: a temporary directory)")
}

type cmakeBuild struct {
	commonBuild

	cmakeCustom       *proj2aci.CmakeCustomizations
	cmakeParamWrapper stringVectorWrapper
}

func newCmakeBuild() build {
	custom := &proj2aci.CmakeCustomizations{}
	return &cmakeBuild{
		commonBuild: commonBuild{
			custom: custom,
			config: custom.GetCommonConfiguration(),
		},
		cmakeCustom: custom,
	}
}

func (build *cmakeBuild) SetupParameters(parameters *flag.FlagSet) {
	// common params
	build.setupCommonParameters(parameters)

	// --binary-dir
	parameters.StringVar(&build.cmakeCustom.Configuration.BinDir, "binary-dir", "", "Look for binaries in this directory (relative to install path, eg passing /usr/local/mysql/bin would look for a binary in <tmpdir>/install/usr/local/mysql/bin")

	// --reuse-src-dir
	parameters.StringVar(&build.cmakeCustom.Configuration.ReuseSrcDir, "reuse-src-dir", "", "Instead of downloading a project, use this path with already downloaded sources")

	// --cmake-param
	build.cmakeParamWrapper.vector = &build.cmakeCustom.Configuration.CmakeParams
	parameters.Var(&build.cmakeParamWrapper, "cmake-param", "Parameters passed to cmake, can be used multiple times")
}
