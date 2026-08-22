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

// SkillLevel is the match skill bracket (for grouping quality metrics).
type SkillLevel string

const (
	SkillHerald   SkillLevel = "herald"
	SkillGuardian SkillLevel = "guardian"
	SkillCrusader SkillLevel = "crusader"
	SkillArchon   SkillLevel = "archon"
	SkillLegend   SkillLevel = "legend"
	SkillAncient  SkillLevel = "ancient"
	SkillDivine   SkillLevel = "divine"
	SkillImmortal SkillLevel = "immortal"
	SkillPro      SkillLevel = "pro"
)

func (s SkillLevel) Label() string {
	return string(s)
}

func (s SkillLevel) order() int {
	switch s {
	case SkillHerald:
		return 1
	case SkillGuardian:
		return 2
	case SkillCrusader:
		return 3
	case SkillArchon:
		return 4
	case SkillLegend:
		return 5
	case SkillAncient:
		return 6
	case SkillDivine:
		return 7
	case SkillImmortal:
		return 8
	case SkillPro:
		return 9
	default:
		return 99
	}
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

type Replay struct {
	ID         string // match ID
	SkillLevel SkillLevel
}

type ReplayHero struct {
	Replay *Replay
	Hero   string
	Role   HeroRole
}

type qualityCase struct {
	Label         string
	Description   string
	ReplayHero    *ReplayHero
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

type bucketKey struct {
	Role HeroRole
	Tag  CaseTag
}

type tagPairKey struct {
	Role  HeroRole
	Label string
}

type bucketStat struct {
	Passed int
	Total  int
	FP     int // unexpected miss (ExpectMiss=false)
	FN     int // expected miss not found (ExpectMiss=true)
}
