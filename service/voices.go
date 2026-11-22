package main

import (
	"fmt"
	"strings"
)

// English voices of kokoro-multi-lang-v1_0. Order = speaker ID in the model (from its speaker_names metadata).
// a = American, b = British; f = female, m = male.
var voices = []string{
	"af_alloy", "af_aoede", "af_bella", "af_heart", "af_jessica", "af_kore", "af_nicole", "af_nova",
	"af_river", "af_sarah", "af_sky", "am_adam", "am_echo", "am_eric", "am_fenrir", "am_liam",
	"am_michael", "am_onyx", "am_puck", "am_santa", "bf_alice", "bf_emma", "bf_isabella", "bf_lily",
	"bm_daniel", "bm_fable", "bm_george", "bm_lewis",
}

// voiceID accepts "af_sarah", "sarah" or "Sarah"; every English voice name is unique without its prefix.
func voiceID(name string) int {
	name = strings.ToLower(name)
	for i, v := range voices {
		if v == name || v[3:] == name {
			return i
		}
	}
	return -1
}

func voicesText() string {
	groups := map[string]string{"af": "American Female", "am": "American Male", "bf": "British Female", "bm": "British Male"}
	var b strings.Builder
	for _, g := range []string{"af", "am", "bf", "bm"} {
		var names []string
		for _, v := range voices {
			if v[:2] == g {
				names = append(names, v)
			}
		}
		fmt.Fprintf(&b, "%s:\n  %s\n", groups[g], strings.Join(names, "  "))
	}
	return strings.TrimSuffix(b.String(), "\n")
}
