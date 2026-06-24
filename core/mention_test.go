package core

import "testing"

func TestIsBotMentioned_WordBoundary(t *testing.T) {
	b := &TelegramBot{botUsername: "myapexbot"}
	cases := []struct {
		text string
		want bool
	}{
		{"apex hlo", true},
		{"hey @myapexbot can you check", true},
		{"@MyApexBot help", true},
		{"@apexclaw what's up", true},
		{"appendix is a word", false},
		{"apexclaw bot is great", true},
		{"Apex Legends is a game", true},
		{"vertex isn't apex either", true},
		{"happexampling", false},
		{"hi", false},
	}
	for _, tc := range cases {
		got := b.isBotMentioned(tc.text)
		if got != tc.want {
			t.Errorf("isBotMentioned(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

func TestIsBotMentioned_NoBotUsername(t *testing.T) {
	b := &TelegramBot{}
	if !b.isBotMentioned("hey apex what's up") {
		t.Error("should still match 'apex' word boundary without botUsername set")
	}
	if b.isBotMentioned("hi") {
		t.Error("'hi' should not be detected as bot mention")
	}
}

func TestLooksLikeBotEcho_RealWorld(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"ApexClaw: The ApexClaw source code is maintained by Amarnath. The public repository link isn't directly accessible.", true},
		{"ApexClaw: Manual Hardware Probe Results\n\nCPU\nlscpu output shows:", true},
		{"ApexClaw:", true},
		{"hey, apex what time is it", false},
		{"normal user message", false},
		{"Apex hlo", false},
		{"  apexclaw:  ", true},
	}
	for _, tc := range cases {
		got := looksLikeBotEcho(tc.text)
		if got != tc.want {
			t.Errorf("looksLikeBotEcho(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}
