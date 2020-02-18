// Copyright 2017 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

// +build

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
)

func main() {
	out, err := os.Create("generated.go")
	if err != nil {
		failf("%v", err)
	}
	defer out.Close()
	data, err := ioutil.ReadFile("../../executor/common.h")
	if err != nil {
		failf("%v", err)
	}
	for _, include := range []string{
		"common_linux.h",
		"common_akaros.h",
		"common_bsd.h",
		"common_fuchsia.h",
		"common_windows.h",
		"common_test.h",
		"common_kvm_amd64.h",
		"common_kvm_arm64.h",
		"common_usb.h",
		"kvm.h",
		"kvm.S.h",
	} {
		contents, err := ioutil.ReadFile("../../executor/" + include)
		if err != nil {
			failf("%v", err)
		}
		replace := []byte("#include \"" + include + "\"")
		if bytes.Index(data, replace) == -1 {
			failf("can't fine %v include", include)
		}
		data = bytes.Replace(data, replace, contents, -1)
	}
	for _, remove := range []string{
		"(\n|^)\\s*//.*",
		"\\s*//.*",
	} {
		data = regexp.MustCompile(remove).ReplaceAll(data, nil)
	}
	fmt.Fprintf(out, "// AUTOGENERATED FILE FROM executor/*.h\n\n")
	fmt.Fprintf(out, "package csource\n\nvar commonHeader = `\n")
	out.Write(data)
	fmt.Fprintf(out, "`\n")
}

func failf(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
	os.Exit(1)
}
