package main

import (
	"flag"
	"fmt"
	"go/token"
	"log"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/exp/apidiff"
	"golang.org/x/exp/slices"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"

	"github.com/icholy/gomajor/internal/fixdocs"
	"github.com/icholy/gomajor/internal/importpaths"
	"github.com/icholy/gomajor/internal/modproxy"
	"github.com/icholy/gomajor/internal/packages"
)

var help = `
GoMajor is a tool for major version upgrades

Usage:

    gomajor <command> [arguments]

The commands are:

    get     upgrade to a major version
    list    list available updates
    path    modify the module path
    diff    list api differences
    help    show this help text
`

func main() {
	flag.Usage = func() {
		fmt.Println(help)
	}
	flag.Parse()
	switch flag.Arg(0) {
	case "get":
		if err := getcmd(flag.Args()[1:]); err != nil {
			log.Fatal(err)
		}
	case "list":
		if err := listcmd(flag.Args()[1:]); err != nil {
			log.Fatal(err)
		}
	case "path":
		if err := pathcmd(flag.Args()[1:]); err != nil {
			log.Fatal(err)
		}
	case "diff":
		if err := diffcmd(flag.Args()[1:]); err != nil {
			log.Fatal(err)
		}
	case "help", "":
		flag.Usage()
	default:
		fmt.Fprintf(os.Stderr, "unrecognized subcommand: %s\n", flag.Arg(0))
		flag.Usage()
	}
}

func listcmd(args []string) error {
	var dir string
	var pre, cached, major bool
	fset := flag.NewFlagSet("list", flag.ExitOnError)
	fset.BoolVar(&pre, "pre", false, "allow non-v0 prerelease versions")
	fset.StringVar(&dir, "dir", ".", "working directory")
	fset.BoolVar(&cached, "cached", true, "only fetch cached content from the module proxy")
	fset.BoolVar(&major, "major", false, "only show newer major versions")
	fset.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: gomajor list")
		fset.PrintDefaults()
	}
	fset.Parse(args)
	dependencies, err := packages.Direct(dir)
	if err != nil {
		return err
	}
	private := os.Getenv("GOPRIVATE")
	for _, dep := range dependencies {
		if module.MatchPrefixPatterns(private, dep.Path) {
			continue
		}
		mod, err := modproxy.Latest(dep.Path, cached)
		if err != nil {
			fmt.Printf("%s: failed: %v\n", dep.Path, err)
			continue
		}
		v := mod.MaxVersion("", pre)
		if major && semver.Compare(semver.Major(v), semver.Major(dep.Version)) <= 0 {
			continue
		}
		if semver.Compare(v, dep.Version) <= 0 {
			continue
		}
		fmt.Printf("%s: %s [latest %v]\n", dep.Path, dep.Version, v)
	}
	return nil
}

func getcmd(args []string) error {
	var dir string
	var rewrite, goget, pre, cached bool
	fset := flag.NewFlagSet("get", flag.ExitOnError)
	fset.BoolVar(&pre, "pre", false, "allow non-v0 prerelease versions")
	fset.BoolVar(&rewrite, "rewrite", true, "rewrite import paths")
	fset.BoolVar(&goget, "get", true, "run go get")
	fset.StringVar(&dir, "dir", ".", "working directory")
	fset.BoolVar(&cached, "cached", true, "only fetch cached content from the module proxy")
	fset.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: gomajor get <pathspec>")
		fset.PrintDefaults()
	}
	fset.Parse(args)
	if fset.NArg() != 1 {
		return fmt.Errorf("missing package spec")
	}
	// resolve the version
	spec, err := modproxy.Resolve(fset.Arg(0), cached, pre)
	if err != nil {
		return err
	}
	// go get
	if goget {
		fmt.Println("go get", spec.String())
		cmd := exec.Command("go", "get", spec.String())
		cmd.Dir = dir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	// rewrite imports
	if !rewrite {
		return nil
	}
	return importpaths.Rewrite(dir, func(pos token.Position, path string) (string, error) {
		_, pkgdir0, ok := packages.SplitPath(spec.ModPrefix, path)
		if !ok {
			return "", importpaths.ErrSkip
		}
		if spec.PackageDir != "" && spec.PackageDir != pkgdir0 {
			return "", importpaths.ErrSkip
		}
		newpath := packages.JoinPath(spec.ModPrefix, spec.Version, pkgdir0)
		if newpath == path {
			return "", importpaths.ErrSkip
		}
		fmt.Printf("%s %s\n", pos, newpath)
		return newpath, nil
	})
}

func pathcmd(args []string) error {
	var dir, version string
	var next, rewrite, docs bool
	fset := flag.NewFlagSet("path", flag.ExitOnError)
	fset.BoolVar(&next, "next", false, "increment the module path version")
	fset.StringVar(&version, "version", "", "set the module path version")
	fset.BoolVar(&rewrite, "rewrite", true, "rewrite import paths")
	fset.StringVar(&dir, "dir", ".", "working directory")
	fset.BoolVar(&docs, "docs", false, "rewrite docs (experimental)")
	fset.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: gomajor path [modpath]")
		fset.PrintDefaults()
	}
	fset.Parse(args)
	// find and parse go.mod
	name, err := packages.FindModFile(dir)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	file, err := modfile.ParseLax(name, data, nil)
	if err != nil {
		return err
	}
	// figure out the new module path
	modpath := fset.Arg(0)
	if modpath == "" {
		modpath = file.Module.Mod.Path
	}
	// find the current version if one wasn't provided
	if version == "" {
		var ok bool
		version, ok = packages.ModMajor(modpath)
		if !ok || version == "" {
			version = "v1"
		}
	}
	// increment the path version
	if next {
		version, err = modproxy.NextMajor(version)
		if err != nil {
			return err
		}
	}
	if !semver.IsValid(version) {
		return fmt.Errorf("invalid version: %q", version)
	}
	// create the new modpath
	modprefix := packages.ModPrefix(modpath)
	oldmodprefix := packages.ModPrefix(file.Module.Mod.Path)
	modpath = packages.JoinPath(modprefix, version, "")
	fmt.Printf("module %s\n", modpath)
	if !rewrite {
		return nil
	}
	// update go.mod
	cmd := exec.Command("go", "mod", "edit", "-module", modpath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	// rewrite import paths
	err = importpaths.Rewrite(dir, func(pos token.Position, path string) (string, error) {
		_, pkgdir, ok := packages.SplitPath(oldmodprefix, path)
		if !ok {
			return "", importpaths.ErrSkip
		}
		newpath := packages.JoinPath(modprefix, version, pkgdir)
		if newpath == path {
			return "", importpaths.ErrSkip
		}
		fmt.Printf("%s %s\n", pos, newpath)
		return newpath, nil
	})
	if err != nil {
		return err
	}
	// rewrite docs
	if docs {
		return fixdocs.RewriteModPath(dir, []string{".md"}, modprefix, version)
	}
	return nil
}

func diffcmd(args []string) error {
	var dir string
	var pre, cached bool
	fset := flag.NewFlagSet("get", flag.ExitOnError)
	fset.BoolVar(&pre, "pre", false, "allow non-v0 prerelease versions")
	fset.StringVar(&dir, "dir", ".", "working directory")
	fset.BoolVar(&cached, "cached", true, "only fetch cached content from the module proxy")
	fset.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: gomajor get <pathspec>")
		fset.PrintDefaults()
	}
	fset.Parse(args)
	if fset.NArg() != 1 {
		return fmt.Errorf("missing package spec")
	}
	// resolve the version
	spec, err := modproxy.Resolve(fset.Arg(0), cached, pre)
	if err != nil {
		return err
	}
	// load all package type info for for resolved version.
	fmt.Printf("loading: %s\n", spec)
	newpkgs, err := packages.LoadModulePackages(spec.Module())
	if err != nil {
		return fmt.Errorf("failed to load type information: %s: %v", spec, err)
	}
	// find the related corresponding local modules
	index, err := packages.LoadIndex(dir)
	if err != nil {
		return err
	}
	related := index.Related(spec.ModPrefix)
	// we only want to diff against packages that we're actually using
	pkgpaths, err := importpaths.List(dir)
	if err != nil {
		return fmt.Errorf("failed to enumerate local imports: %v", err)
	}
	for _, pkgpath := range pkgpaths {
		mod, ok := index.Lookup(pkgpath)
		// check if it belongs to the module we're looking for
		if !ok || !slices.Contains(related, mod) {
			continue
		}
		// if the spec contains a package directory, only diff that and its subdirs.
		if spec.PackageDir != "" && !strings.HasPrefix(pkgpath, spec.PackagePath()) {
			continue
		}
		// find the corresponding package in the temp module
		_, pkgdir, _ := packages.SplitPath(spec.ModPrefix, pkgpath)
		newpkgpath := packages.JoinPath(spec.ModPrefix, spec.Version, pkgdir)
		var newpkg *packages.Package
		for _, pkg := range newpkgs {
			if pkg.PkgPath == newpkgpath {
				newpkg = pkg
				break
			}
		}
		if newpkg == nil {
			fmt.Printf("package %s: does not exist in %s\n", newpkgpath, spec.Version)
			continue
		}
		// load the local package type info
		oldpkg, err := packages.LoadPackage(dir, pkgpath)
		if err != nil {
			fmt.Printf("package %s: error %v\n", pkgpath, err)
			continue
		}
		// diff it
		report := apidiff.Changes(oldpkg.Types, newpkg.Types)
		incompatible := false
		for _, ch := range report.Changes {
			if !ch.Compatible {
				incompatible = true
				break
			}
		}
		if incompatible {
			fmt.Printf("package %s:\n", pkgpath)
			if err := report.TextIncompatible(os.Stdout, false); err != nil {
				return err
			}
		}
	}
	return nil
}
