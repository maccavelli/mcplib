package selfupdate

import (
	"errors"
	"fmt"
	"testing"
)

func TestStrictVersionPolicyValidate(t *testing.T) {
	p := NewStrictVersionPolicy()
	tests := []struct {
		name    string
		tag     string
		wantErr bool
	}{
		{name: "stable", tag: "v1.2.3"},
		{name: "zero patch", tag: "v0.0.0"},
		{name: "two-digit", tag: "v10.20.30"},
		{name: "incomplete major.minor", tag: "v1.2", wantErr: true},
		{name: "major only", tag: "v1", wantErr: true},
		{name: "missing v", tag: "1.2.3", wantErr: true},
		{name: "uppercase V", tag: "V1.2.3", wantErr: true},
		{name: "leading zero major", tag: "v01.2.3", wantErr: true},
		{name: "leading zero minor", tag: "v1.02.3", wantErr: true},
		{name: "leading zero patch", tag: "v1.2.03", wantErr: true},
		{name: "prerelease", tag: "v1.2.3-alpha", wantErr: true},
		{name: "prerelease dot", tag: "v1.2.3-alpha.1", wantErr: true},
		{name: "build metadata", tag: "v1.2.3+build.1", wantErr: true},
		{name: "prerelease and build", tag: "v1.2.3-alpha+build", wantErr: true},
		{name: "four components", tag: "v1.2.3.4", wantErr: true},
		{name: "legacy BASE.N", tag: "0.15.3.12", wantErr: true},
		{name: "legacy BASE.N with v", tag: "v0.15.3.12", wantErr: true},
		{name: "empty prerelease", tag: "v1.2.3-", wantErr: true},
		{name: "illegal identifier", tag: "v1.2.3-foo_bar", wantErr: true},
		{name: "empty", tag: "", wantErr: true},
		{name: "v only", tag: "v", wantErr: true},
		{name: "local garbage", tag: "dev", wantErr: true},
		{name: "whitespace", tag: " v1.2.3", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.Validate(tt.tag)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Validate(%q) succeeded, want error", tt.tag)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate(%q) = %v", tt.tag, err)
			}
		})
	}
}

func TestStrictVersionPolicyCompare(t *testing.T) {
	p := NewStrictVersionPolicy()
	cmp, err := p.Compare("v1.2.3", "v1.2.4")
	if err != nil {
		t.Fatal(err)
	}
	if cmp >= 0 {
		t.Fatalf("Compare(v1.2.3, v1.2.4) = %d, want negative", cmp)
	}
	cmp, err = p.Compare("v2.0.0", "v1.9.9")
	if err != nil {
		t.Fatal(err)
	}
	if cmp <= 0 {
		t.Fatalf("Compare(v2.0.0, v1.9.9) = %d, want positive", cmp)
	}
	cmp, err = p.Compare("v1.0.0", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if cmp != 0 {
		t.Fatalf("Compare equal = %d, want 0", cmp)
	}
	if _, err := p.Compare("v1.2", "v1.2.0"); err == nil {
		t.Fatal("Compare accepted incomplete version")
	}
	if _, err := p.Compare("v1.2.3-alpha", "v1.2.3"); err == nil {
		t.Fatal("Compare accepted prerelease")
	}
}

func TestClassifyOperation(t *testing.T) {
	p := NewStrictVersionPolicy()
	tests := []struct {
		name       string
		req        Request
		selected   string
		fromLatest bool
		want       Operation
		wantErr    error
	}{
		{
			name:     "upgrade",
			req:      Request{CurrentBuild: ReleaseBuild, CurrentVersion: "v1.0.0"},
			selected: "v1.1.0",
			want:     OperationUpgrade,
		},
		{
			name:     "upgrade ignores force",
			req:      Request{CurrentBuild: ReleaseBuild, CurrentVersion: "v1.0.0", Force: true},
			selected: "v1.1.0",
			want:     OperationUpgrade,
		},
		{
			name:     "current equal no force",
			req:      Request{CurrentBuild: ReleaseBuild, CurrentVersion: "v1.0.0"},
			selected: "v1.0.0",
			want:     OperationNone,
		},
		{
			name:     "same version force apply",
			req:      Request{CurrentBuild: ReleaseBuild, CurrentVersion: "v1.0.0", Force: true},
			selected: "v1.0.0",
			want:     OperationReinstall,
		},
		{
			name:     "exact rollback apply",
			req:      Request{CurrentBuild: ReleaseBuild, CurrentVersion: "v1.1.0", TargetVersion: "v1.0.0"},
			selected: "v1.0.0",
			want:     OperationRollback,
		},
		{
			name:     "exact rollback check",
			req:      Request{CurrentBuild: ReleaseBuild, CurrentVersion: "v1.1.0", TargetVersion: "v1.0.0", CheckOnly: true},
			selected: "v1.0.0",
			want:     OperationRollback,
		},
		{
			name:       "lower latest",
			req:        Request{CurrentBuild: ReleaseBuild, CurrentVersion: "v1.1.0"},
			selected:   "v1.0.0",
			fromLatest: true,
			wantErr:    errLatestOlder,
		},
		{
			name:     "local check",
			req:      Request{CurrentBuild: LocalBuild, CurrentVersion: "dev", CheckOnly: true},
			selected: "v1.0.0",
			want:     OperationReplaceLocal,
		},
		{
			name:     "local apply without force",
			req:      Request{CurrentBuild: LocalBuild, CurrentVersion: "dev"},
			selected: "v1.0.0",
			wantErr:  errForceRequired,
		},
		{
			name:     "local force apply",
			req:      Request{CurrentBuild: LocalBuild, CurrentVersion: "not-a-version", Force: true},
			selected: "v1.0.0",
			want:     OperationReplaceLocal,
		},
		{
			name:     "local garbage is not ordered",
			req:      Request{CurrentBuild: LocalBuild, CurrentVersion: "v9.9.9", CheckOnly: true},
			selected: "v1.0.0",
			want:     OperationReplaceLocal,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := classifyOperation(p, tt.req, tt.selected, tt.fromLatest)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestValidateRequest(t *testing.T) {
	valid := Request{
		Product:        "prepare-commit-msg",
		CurrentVersion: "v1.2.0",
		CurrentBuild:   ReleaseBuild,
	}
	t.Run("ok", func(t *testing.T) {
		if err := validateRequest(valid); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("unknown build kind", func(t *testing.T) {
		req := valid
		req.CurrentBuild = BuildKind(9)
		if err := validateRequest(req); err == nil {
			t.Fatal("accepted unknown build kind")
		}
	})
	t.Run("zero build kind", func(t *testing.T) {
		req := valid
		req.CurrentBuild = BuildUnknown
		if err := validateRequest(req); err == nil {
			t.Fatal("accepted unknown zero build kind")
		}
	})
	t.Run("partial platform", func(t *testing.T) {
		req := valid
		req.Platform.OS = "linux"
		if err := validateRequest(req); err == nil {
			t.Fatal("accepted partial platform")
		}
	})
	t.Run("check plus yes", func(t *testing.T) {
		req := valid
		req.CheckOnly = true
		req.Yes = true
		if err := validateRequest(req); err == nil {
			t.Fatal("accepted --check --yes")
		}
	})
	t.Run("check plus force", func(t *testing.T) {
		req := valid
		req.CheckOnly = true
		req.Force = true
		if err := validateRequest(req); err == nil {
			t.Fatal("accepted --check --force")
		}
	})
	t.Run("invalid product", func(t *testing.T) {
		req := valid
		req.Product = "../evil"
		if err := validateRequest(req); err == nil {
			t.Fatal("accepted invalid product")
		}
	})
	t.Run("release current not a tag", func(t *testing.T) {
		req := valid
		req.CurrentVersion = "dev"
		if err := validateRequest(req); err == nil {
			t.Fatal("accepted local identity as release")
		}
	})
	t.Run("local current not ordered", func(t *testing.T) {
		req := valid
		req.CurrentBuild = LocalBuild
		req.CurrentVersion = "definitely not semver"
		if err := validateRequest(req); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("invalid exact target", func(t *testing.T) {
		req := valid
		req.TargetVersion = "v1.2.3-rc.1"
		if err := validateRequest(req); err == nil {
			t.Fatal("accepted prerelease target")
		}
	})
}

func TestExitCode(t *testing.T) {
	if got := ExitCode(Result{}, nil); got != 0 {
		t.Fatalf("nil err = %d, want 0", got)
	}
	if got := ExitCode(Result{Declined: true}, nil); got != 0 {
		t.Fatalf("declined = %d, want 0", got)
	}
	if got := ExitCode(Result{Checked: true}, ErrUpdateAvailable); got != 10 {
		t.Fatalf("available = %d, want 10", got)
	}
	wrapped := fmt.Errorf("selfupdate: demo: %w", ErrUpdateAvailable)
	if got := ExitCode(Result{}, wrapped); got != 10 {
		t.Fatalf("wrapped available = %d, want 10", got)
	}
	if got := ExitCode(Result{}, ErrIntegrity); got != 1 {
		t.Fatalf("integrity = %d, want 1", got)
	}
}

func TestEnumString(t *testing.T) {
	if ReleaseBuild.String() != "release" {
		t.Fatalf("ReleaseBuild = %q", ReleaseBuild.String())
	}
	if OperationRollback.String() != "rollback" {
		t.Fatalf("OperationRollback = %q", OperationRollback.String())
	}
	if EventInstalling.String() != "installing" {
		t.Fatalf("EventInstalling = %q", EventInstalling.String())
	}
	if BuildKind(42).String() != "buildkind(42)" {
		t.Fatalf("unknown BuildKind = %q", BuildKind(42).String())
	}
}

func TestDefaultLimits(t *testing.T) {
	l := DefaultLimits()
	if l.ReleaseJSON != 2<<20 || l.ErrorBody != 64<<10 || l.Manifest != 1<<20 || l.Executable != 512<<20 {
		t.Fatalf("DefaultLimits() = %+v", l)
	}
}
