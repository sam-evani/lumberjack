package worktree

import "testing"

func TestParsePorcelainParsesAttachedWorktrees(t *testing.T) {
	input := []byte("worktree /Users/sam.evani/co/backend\nHEAD 7a22ee17c8d5c65be708c408a114772a87005a7f\nbranch refs/heads/master\n\nworktree /Users/sam.evani/co/backend/.worktrees/feature\nHEAD 1262bbf843c56f8964e722b84d5bcc6bc7216e01\nbranch refs/heads/sam-evani/feature\n\n")

	got, err := ParsePorcelain(input)
	if err != nil {
		t.Fatalf("ParsePorcelain returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(ParsePorcelain) = %d, want 2", len(got))
	}
	if got[0].Path != "/Users/sam.evani/co/backend" {
		t.Fatalf("first path = %q", got[0].Path)
	}
	if got[0].Branch != "master" {
		t.Fatalf("first branch = %q", got[0].Branch)
	}
	if got[1].Branch != "sam-evani/feature" {
		t.Fatalf("second branch = %q", got[1].Branch)
	}
}

func TestParsePorcelainParsesDetachedAndPrunable(t *testing.T) {
	input := []byte("worktree /tmp/repo-detached\nHEAD abc123\n\nworktree /tmp/missing\nHEAD def456\nbranch refs/heads/missing-branch\nprunable gitdir file points to non-existent location\n\n")

	got, err := ParsePorcelain(input)
	if err != nil {
		t.Fatalf("ParsePorcelain returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(ParsePorcelain) = %d, want 2", len(got))
	}
	if !got[0].Detached {
		t.Fatalf("first worktree Detached = false, want true")
	}
	if !got[1].Prunable {
		t.Fatalf("second worktree Prunable = false, want true")
	}
	if got[1].PrunableReason != "gitdir file points to non-existent location" {
		t.Fatalf("PrunableReason = %q", got[1].PrunableReason)
	}
}

func TestParsePorcelainParsesLockedWorktrees(t *testing.T) {
	input := []byte("worktree /tmp/locked-no-reason\nHEAD abc123\nbranch refs/heads/locked-no-reason\nlocked\n\nworktree /tmp/locked-with-reason\nHEAD def456\nbranch refs/heads/locked-with-reason\nlocked in use by deploy\n\n")

	got, err := ParsePorcelain(input)
	if err != nil {
		t.Fatalf("ParsePorcelain returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(ParsePorcelain) = %d, want 2", len(got))
	}
	if !got[0].Locked {
		t.Fatalf("first worktree Locked = false, want true")
	}
	if got[0].LockedReason != "" {
		t.Fatalf("first LockedReason = %q, want empty", got[0].LockedReason)
	}
	if !got[1].Locked {
		t.Fatalf("second worktree Locked = false, want true")
	}
	if got[1].LockedReason != "in use by deploy" {
		t.Fatalf("second LockedReason = %q", got[1].LockedReason)
	}
}

func TestParsePorcelainParsesKeyOnlyDetachedAndBare(t *testing.T) {
	input := []byte("worktree /tmp/detached\nHEAD abc123\ndetached\n\nworktree /tmp/bare\nHEAD def456\nbranch refs/heads/main\nbare\n\n")

	got, err := ParsePorcelain(input)
	if err != nil {
		t.Fatalf("ParsePorcelain returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(ParsePorcelain) = %d, want 2", len(got))
	}
	if !got[0].Detached {
		t.Fatalf("first worktree Detached = false, want true")
	}
	if !got[1].Bare {
		t.Fatalf("second worktree Bare = false, want true")
	}
}

func TestParsePorcelainParsesBareWithoutBranch(t *testing.T) {
	input := []byte("worktree /tmp/bare.git\nbare\n\n")

	got, err := ParsePorcelain(input)
	if err != nil {
		t.Fatalf("ParsePorcelain returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(ParsePorcelain) = %d, want 1", len(got))
	}
	if !got[0].Bare {
		t.Fatalf("Bare = false, want true")
	}
	if got[0].Detached {
		t.Fatalf("Detached = true, want false")
	}
}
