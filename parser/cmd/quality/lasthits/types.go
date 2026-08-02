package main

// HeroRole is the player's role in the match (for grouping quality metrics).
type HeroRole string

const (
	RoleSupport HeroRole = "support"
	RoleCore    HeroRole = "core"
)

// CaseType categorizes what heuristic scenario is being checked.
type CaseType string

const (
	CaseMissedDeny            CaseType = "missed_deny"
	CaseMissedLastHit         CaseType = "missed_last_hit"
	CaseHitCreepTooEarly      CaseType = "hit_creep_too_early"
	CaseHitCreepTooLate       CaseType = "hit_creep_too_late"
	CaseUsedSpellTooEarly     CaseType = "used_spell_too_early"
	CaseUsedSpellTooLate      CaseType = "used_spell_too_late"
	CaseCreepDiedToAOESpell   CaseType = "creep_died_to_aoe_spell"
	CaseCreepDiedToRandomProc CaseType = "creep_died_to_random_proc"
)

var caseTypeLabels = map[CaseType]string{
	CaseMissedDeny:            "missed deny",
	CaseMissedLastHit:         "missed last hit",
	CaseHitCreepTooEarly:      "hit creep too early",
	CaseHitCreepTooLate:       "hit creep too late",
	CaseUsedSpellTooEarly:     "used spell too early",
	CaseUsedSpellTooLate:      "used spell too late",
	CaseCreepDiedToAOESpell:   "creep died to AOE spell",
	CaseCreepDiedToRandomProc: "creep died to random proc effect",
}

func (t CaseType) Label() string {
	if s, ok := caseTypeLabels[t]; ok {
		return s
	}
	return string(t)
}

func (r HeroRole) Label() string {
	return string(r)
}

type qualityCase struct {
	Label         string
	Description   string
	Replay        string // match ID
	Hero          string
	HeroRole      HeroRole
	CaseType      CaseType
	From          float32
	To            float32
	CreepContains string
	ExpectMiss    bool
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
	Type CaseType
}

type bucketStat struct {
	Passed int
	Total  int
}
