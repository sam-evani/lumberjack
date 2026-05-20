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
