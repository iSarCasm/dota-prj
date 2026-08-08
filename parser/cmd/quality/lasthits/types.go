package main

// HeroRole is the player's role in the match (for grouping quality metrics).
type HeroRole string

const (
	RoleSupport HeroRole = "support"
	RoleCore    HeroRole = "core"
)

func (r HeroRole) Label() string {
	return string(r)
}

// CaseTag labels a quality case for filtering and grouping.
type CaseTag string

const (
	TagAutoAttack CaseTag = "auto-attack"
	TagSpell      CaseTag = "spell"
	TagAOE        CaseTag = "aoe"
	TagDeny        CaseTag = "deny"
	TagUncontested CaseTag = "uncontested"
	TagTooEarly    CaseTag = "too-early"
	TagTooLate    CaseTag = "too-late"
)

type qualityCase struct {
	Label         string
	Description   string
	Replay        string // match ID
	Hero          string
	HeroRole      HeroRole
	From          float32
	To            float32
	CreepContains string
	ExpectMiss    bool
	Tags          []CaseTag
}

type caseResult struct {
	Case   qualityCase
	Pass   bool
	Detail string
}

type groupKey struct {
	Replay string
	Hero   string
}

type bucketKey struct {
	Role HeroRole
	Tag  CaseTag
}

type bucketStat struct {
	Passed int
	Total  int
}
