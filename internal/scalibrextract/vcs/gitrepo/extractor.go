// Package gitrepo provides an extractor for git repositories and submodules.
package gitrepo

import (
	"context"
	"path"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/google/osv-scalibr/extractor"
	"github.com/google/osv-scalibr/extractor/filesystem"
	"github.com/google/osv-scalibr/inventory"
	"github.com/google/osv-scalibr/plugin"
	"github.com/google/osv-scalibr/purl"
)

const (
	// Name is the unique name of this extractor.
	Name = "vcs/gitrepo"
)

type Config struct {
	IncludeRootGit bool
	Disabled       bool
}

// Extractor extracts git repository hashes including submodule hashes.
// This extractor will not return an error, and will just return no results if we fail to extract
type Extractor struct {
	IncludeRootGit bool
	Disabled       bool
}

func getCommitSHA(repo *git.Repository) (string, error) {
	head, err := repo.Head()
	if err != nil {
		return "", err
	}

	return head.Hash().String(), nil
}

func getSubmodules(repo *git.Repository) (submodules []*git.SubmoduleStatus, err error) {
	worktree, err := repo.Worktree()
	if err != nil {
		return nil, err
	}
	ss, err := worktree.Submodules()
	if err != nil {
		return nil, err
	}
	for _, s := range ss {
		status, err := s.Status()
		if err != nil {
			continue
		}
		submodules = append(submodules, status)
	}

	return submodules, nil
}

func createCommitQueryInventory(commit string, location string) *extractor.Package {
	return &extractor.Package{
		SourceCode: &extractor.SourceCodeIdentifier{
			Commit: commit,
		},
		Locations: []string{location},
	}
}

// Name of the extractor.
func (e *Extractor) Name() string { return Name }

// Version of the extractor.
func (e *Extractor) Version() int { return 0 }

// Requirements of the extractor.
func (e *Extractor) Requirements() *plugin.Capabilities {
	return &plugin.Capabilities{
		ExtractFromDirs: true,
	}
}

// FileRequired returns true for git repositories .git dirs
func (e *Extractor) FileRequired(fapi filesystem.FileAPI) bool {
	if e.Disabled {
		return false
	}

	if filepath.Base(fapi.Path()) != ".git" {
		return false
	}

	// Stat costs performance, so perform it after the name check
	stat, err := fapi.Stat()
	if err != nil {
		return false
	}

	return stat.IsDir()
}

// Extract extracts git commits from HEAD and from submodules
func (e *Extractor) Extract(_ context.Context, input *filesystem.ScanInput) (inventory.Inventory, error) {
	// todo: maybe we should return an error instead? need to double check we're always using FileRequired correctly first
	if e.Disabled {
		return inventory.Inventory{}, nil
	}

	// The input path is the .git directory, but git.PlainOpen expects the actual directory containing the .git dir.
	// So call filepath.Dir to get the parent path
	// Assume this is fully on a real filesystem
	// TODO: Make this support virtual filesystems
	repo, err := git.PlainOpen(path.Join(input.Root, filepath.Dir(input.Path)))
	if err != nil {
		return inventory.Inventory{}, err
	}

	var inv inventory.Inventory

	if e.IncludeRootGit {
		commitSHA, err := getCommitSHA(repo)

		// If error is not nil, then ignore this and continue, as it is not fatal.
		// The error could be because there are no commits in the repository
		if err == nil {
			inv.Packages = append(inv.Packages, createCommitQueryInventory(commitSHA, input.Path))
		}
	}

	// If we can't get submodules, just return with what we have.
	submodules, err := getSubmodules(repo)
	if err != nil {
		return inv, err
	}

	for _, s := range submodules {
		// r.Infof("Scanning submodule %s at commit %s\n", s.Path, s.Expected.String())
		inv.Packages = append(inv.Packages, createCommitQueryInventory(s.Expected.String(), path.Join(input.Path, s.Path)))
	}

	return inv, nil
}

// ToPURL converts an inventory created by this extractor into a PURL.
func (e *Extractor) ToPURL(_ *extractor.Package) *purl.PackageURL {
	return nil
}

// Ecosystem returns an empty string as all inventories are commit hashes
func (e *Extractor) Ecosystem(_ *extractor.Package) string {
	return ""
}

var _ filesystem.Extractor = &Extractor{}

type configurable interface {
	Configure(config Config)
}

func (e *Extractor) Configure(config Config) {
	e.IncludeRootGit = config.IncludeRootGit
	e.Disabled = config.Disabled
}

var _ configurable = &Extractor{}

func Configure(extrac extractor.Extractor, config Config) {
	us, ok := extrac.(configurable)

	if ok {
		us.Configure(config)
	}
}
