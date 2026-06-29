package cli

import "testing"

func TestAgentAssetName(t *testing.T) {
	got := agentAssetName("0.1.0", "darwin", "arm64")
	want := "fallbakit-agent_0.1.0_darwin_arm64.tar.gz"
	if got != want {
		t.Fatalf("agentAssetName = %q, want %q", got, want)
	}
}

func TestChecksumFor(t *testing.T) {
	sums := "abc123  fallbakit-agent_0.1.0_linux_amd64.tar.gz\n" +
		"DEF456  fallbakit-agent_0.1.0_darwin_arm64.tar.gz\n"
	if got := checksumFor(sums, "fallbakit-agent_0.1.0_linux_amd64.tar.gz"); got != "abc123" {
		t.Fatalf("linux checksum = %q, want abc123", got)
	}
	// lowercased
	if got := checksumFor(sums, "fallbakit-agent_0.1.0_darwin_arm64.tar.gz"); got != "def456" {
		t.Fatalf("darwin checksum = %q, want def456", got)
	}
	if got := checksumFor(sums, "missing.tar.gz"); got != "" {
		t.Fatalf("missing checksum = %q, want empty", got)
	}
}
