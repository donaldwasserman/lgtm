package main

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// gitTrees holds the two directories extracted from a git checkout.
type gitTrees struct {
	base, head string
	root       string
}

func (t gitTrees) cleanup() { os.RemoveAll(t.root) }

// extractTrees resolves the merge base of baseRef and headRef in repo and
// writes both commits' trees to a temporary directory.
//
// It never fetches. A shallow clone or a missing base ref is reported as an
// error, not repaired: the caller owns the checkout, and a verdict computed
// against a guessed base would be worse than none.
func extractTrees(repo, baseRef, headRef string) (gitTrees, error) {
	head, err := resolveCommit(repo, headRef)
	if err != nil {
		return gitTrees{}, err
	}
	if _, err := resolveCommit(repo, baseRef); err != nil {
		return gitTrees{}, fmt.Errorf("%w (fetch the base branch before running lgtm)", err)
	}
	mergeBase, err := git(repo, "merge-base", baseRef, head)
	if err != nil {
		return gitTrees{}, fmt.Errorf("no merge base between %q and %q; the clone is probably shallow, so fetch full history (e.g. fetch-depth: 0): %w", baseRef, headRef, err)
	}

	root, err := os.MkdirTemp("", "lgtm-")
	if err != nil {
		return gitTrees{}, err
	}
	t := gitTrees{
		base: filepath.Join(root, "base"),
		head: filepath.Join(root, "head"),
		root: root,
	}
	if err := archive(repo, mergeBase, t.base); err != nil {
		t.cleanup()
		return gitTrees{}, err
	}
	if err := archive(repo, head, t.head); err != nil {
		t.cleanup()
		return gitTrees{}, err
	}
	return t, nil
}

func resolveCommit(repo, ref string) (string, error) {
	sha, err := git(repo, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("cannot resolve %q to a commit in %s", ref, repo)
	}
	return sha, nil
}

// git runs a git subcommand in repo and returns its trimmed stdout.
func git(repo string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("git %s: %s", args[0], msg)
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// archive writes the tree of commit into dir. Only regular files and
// directories are written: a symlink in the tree could point outside dir, and
// the scanner has no use for one.
func archive(repo, commit, dir string) error {
	var stderr bytes.Buffer
	cmd := exec.Command("git", "-C", repo, "archive", "--format=tar", commit)
	cmd.Stderr = &stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("git archive: %w", err)
	}
	extractErr := untar(pipe, dir)
	// Drain so git is not blocked writing to a pipe nobody reads.
	_, _ = io.Copy(io.Discard, pipe)
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("git archive %s: %s", commit, strings.TrimSpace(stderr.String()))
	}
	return extractErr
}

func untar(r io.Reader, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading git archive: %w", err)
		}
		name := filepath.Clean(hdr.Name)
		if name == "." || filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			continue
		}
		target := filepath.Join(dir, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			_, err = io.Copy(f, tr)
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
		}
	}
}
