package main

import (
	"strings"
)

type replayList []string

func (r *replayList) String() string {
	return strings.Join(*r, ",")
}

func (r *replayList) Set(v string) error {
	*r = append(*r, v)
	return nil
}

func collectReplayPaths(replays replayList, args []string) []string {
	all := append([]string(nil), replays...)
	all = append(all, args...)

	seen := make(map[string]struct{})
	out := make([]string, 0, len(all))
	for _, r := range all {
		if r == "" {
			continue
		}
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	return out
}
