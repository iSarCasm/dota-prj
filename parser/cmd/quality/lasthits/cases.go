package main

func gameClock(min, sec int) float32 {
	return float32(min*60 + sec)
}

// qualityCases — edit freely; not part of the build gate.
var qualityCases = []qualityCase{
	{
		Label:         "2:46 missed deny on flagbearer",
		Description:   "Warlock right-clicks ally flagbearer low; Huskar kills it",
		Replay:        "8915936762",
		Hero:          "Warlock",
		HeroRole:      RoleSupport,
		CaseType:      CaseMissedDeny,
		From:          gameClock(2, 44),
		To:            gameClock(2, 47),
		CreepContains: "badguys_flagbearer",
		ExpectMiss:    true,
	},
	{
		Label:         "2:52 creep dies to Fatal Bonds",
		Description:   "Allied melee creep death from Fatal Bonds spread damage",
		Replay:        "8915936762",
		Hero:          "Warlock",
		HeroRole:      RoleSupport,
		CaseType:      CaseCreepDiedToRandomProc,
		From:          gameClock(2, 51),
		To:            gameClock(2, 53),
		CreepContains: "goodguys_melee",
		ExpectMiss:    false,
	},
	{
		Label:         "2:55 creep dies to Fatal Bonds",
		Description:   "Allied melee creep death from Fatal Bonds spread damage",
		Replay:        "8915936762",
		Hero:          "Warlock",
		HeroRole:      RoleSupport,
		CaseType:      CaseCreepDiedToRandomProc,
		From:          gameClock(2, 54),
		To:            gameClock(2, 56),
		CreepContains: "goodguys_melee",
		ExpectMiss:    false,
	},
	{
		Label:         "2:58 creep dies to Fatal Bonds",
		Description:   "Allied ranged creep death from Fatal Bonds spread damage",
		Replay:        "8915936762",
		Hero:          "Warlock",
		HeroRole:      RoleSupport,
		CaseType:      CaseCreepDiedToRandomProc,
		From:          gameClock(2, 57),
		To:            gameClock(2, 59),
		CreepContains: "goodguys_ranged",
		ExpectMiss:    false,
	},
}
