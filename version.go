// Copyright 2017-2023 Fortio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package version wraps golang BuildInfo for easy versioning of
// go binaries (installed or built through go install).
package main

import (
	"fmt"
	"log"
	"runtime"
	"runtime/debug"
	"strings"
)

// FromBuildInfo can be called by other programs to get their version strings (short,long and full)
// automatically added by go 1.18+ when doing `go install project@vX.Y.Z`.
func FromBuildInfo() (short, long, full string) {
	return FromBuildInfoPath("")
}

func normalizeVersion(version string) string {
	// skip leading v, assumes the project use `vX.Y.Z` tags.
	short := strings.TrimLeft(version, "v")
	// '(devel)' messes up the release-tests paths
	if short == "(devel)" || short == "" {
		short = "dev"
	}
	return short
}

func getVersion(binfo *debug.BuildInfo, path string) (short, sum, mainPath, base string, isMain bool) {
	mainPath = binfo.Main.Path
	base = normalizeVersion(binfo.Main.Version)
	if path == "" || path == mainPath {
		sum = binfo.Main.Sum
		short = base
		isMain = true
		return
	}
	// try to find the right module in deps
	short = path + " not found in buildinfo"
	for _, m := range binfo.Deps {
		if path == m.Path {
			short = strings.TrimLeft(m.Version, "v")
			sum = m.Sum
			return
		}
	}
	return
}

// FromBuildInfoPath returns the version of as specific module if that module isn't already the main one.
// Can be used by any other library to extract their own running version.
// It will also indicate the containing binary's version if the module is not the main one.
func FromBuildInfoPath(path string) (short, long, full string) {
	binfo, ok := debug.ReadBuildInfo()
	if !ok {
		full = "version module error, no build info"
		log.Print("Error calling debug.ReadBuildInfo() for version module")
		return
	}
	short, sum, mainPath, base, isMain := getVersion(binfo, path)
	long = short + " " + sum + " " + binfo.GoVersion + " " + runtime.GOARCH + " " + runtime.GOOS
	if !isMain {
		long = long + " (in " + mainPath + " " + base + ")"
	}
	full = fmt.Sprintf("%s\n%v", long, binfo.String())
	return
}
