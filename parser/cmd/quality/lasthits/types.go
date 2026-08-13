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
	TagAutoAttack  CaseTag = "auto-attack"  // Indicates that we tried to last hit with auto attack
	TagSpell       CaseTag = "spell"        // We tried to last hist with a spell
	TagAOE         CaseTag = "aoe"          // We could not last hit due to AOE calculation
	TagDeny        CaseTag = "deny"         // Tried to LH an ally creep
	TagUncontested CaseTag = "uncontested"  // We didnt even try
	TagTooEarly    CaseTag = "too-early"    // Damaged too early
	TagTooLate     CaseTag = "too-late"     // Started attack/spell too late so it didnt reach the target
	TagTower       CaseTag = "tower"        // Creep was being damaged by a tower
	TagFreeFarming CaseTag = "free-farming" // There are no enemies nearby
	TagIllusion    CaseTag = "illusion"     // Attempted LH with an Illusion
	TagGlyph       CaseTag = "glyph"        // Enemy team used glyph of fortification before creep was about to die
	TagOutplayed   CaseTag = "outplayed"    // Equal chances that we lost despite having higher damage
	TagImpossible  CaseTag = "impossilbe"   // Impossible last hit
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
