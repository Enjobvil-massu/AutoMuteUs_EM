package bot

import (
	"errors"

	"github.com/automuteus/automuteus/v8/pkg/game"
	"github.com/bwmarrin/discordgo"
)

var errHostTalkRevisionOverflow = errors.New("host talk revision exhausted")
var errHostTalkGameStateUnavailable = errors.New("host talk game state unavailable")
var errHostTalkLockUnavailable = errors.New("host talk game-state lock unavailable")
var errHostTalkPersistFailed = errors.New("host talk game-state persistence failed")
var errHostTalkVoiceSnapshotStale = errors.New("host talk voice snapshot stale")

// isHostTalkAuthorized uses Discord identity and permissions, not EM bot-admin settings.
// The caller is responsible for obtaining permissions for this user in the correct guild.
func isHostTalkAuthorized(leaderID, userID, guildOwnerID string, permissions int64) bool {
	if userID == "" {
		return false
	}
	return (leaderID != "" && userID == leaderID) ||
		(guildOwnerID != "" && userID == guildOwnerID) ||
		permissions&discordgo.PermissionAdministrator != 0
}

// nextHostTalkState sets an explicit mode without mutating stored state.
// A revision overflow leaves both the current mode and revision unchanged.
func nextHostTalkState(currentMode bool, currentRevision uint64, requestedMode bool) (newMode bool, newRevision uint64, changed bool, err error) {
	if currentMode == requestedMode {
		return currentMode, currentRevision, false, nil
	}
	if currentRevision == ^uint64(0) {
		return currentMode, currentRevision, false, errHostTalkRevisionOverflow
	}
	return requestedMode, currentRevision + 1, true, nil
}

// planHostTalkGameState creates the candidate state for an explicit HostTalk ON/OFF request.
// It never mutates current. Persistence is deliberately handled by a separate locked transaction layer.
// HostTalkManagedUsers is preserved across mode changes so OFF reconciliation can clean users that HostTalk managed.
func planHostTalkGameState(current *GameState, requestedMode bool) (*GameState, bool, error) {
	if current == nil {
		return nil, false, errHostTalkGameStateUnavailable
	}

	candidate := *current
	candidate.GameStateMsg = current.GameStateMsg

	newMode, newRevision, changed, err := nextHostTalkState(
		current.GameStateMsg.HostTalkMode,
		current.GameStateMsg.HostTalkRevision,
		requestedMode,
	)
	if err != nil {
		return &candidate, false, err
	}
	if !changed {
		return &candidate, false, nil
	}

	candidate.GameStateMsg.HostTalkMode = newMode
	candidate.GameStateMsg.HostTalkRevision = newRevision
	return &candidate, true, nil
}

// commitHostTalkGameState persists a planned HostTalk transition before exposing it as committed.
// current is never mutated. A persistence failure therefore leaves the caller with the original state.
func commitHostTalkGameState(current *GameState, requestedMode bool, persist func(*GameState) error) (*GameState, bool, error) {
	candidate, changed, err := planHostTalkGameState(current, requestedMode)
	if err != nil {
		return current, false, err
	}
	if !changed {
		return current, false, nil
	}
	if persist == nil {
		return current, false, errHostTalkPersistFailed
	}
	if err := persist(candidate); err != nil {
		return current, false, errors.Join(errHostTalkPersistFailed, err)
	}
	return candidate, true, nil
}

// setHostTalkMode serializes an explicit ON/OFF transition with the existing Redis game-state lock.
// It refuses to create a GameState when the requested game no longer exists.
func (bot *Bot) setHostTalkMode(gsr GameStateRequest, requestedMode bool) (*GameState, bool, error) {
	if bot == nil || bot.RedisInterface == nil {
		return nil, false, errHostTalkGameStateUnavailable
	}

	// Reject stale/nonexistent games before attempting the existing-only lock path.
	if bot.RedisInterface.getDiscordGameStateKey(gsr) == "" {
		return nil, false, errHostTalkGameStateUnavailable
	}

	lock, current := bot.RedisInterface.getExistingDiscordGameStateAndLock(gsr)
	if lock == nil || current == nil {
		return nil, false, errHostTalkLockUnavailable
	}
	defer lock.Release(ctx)

	return commitHostTalkGameState(
		current,
		requestedMode,
		bot.RedisInterface.persistExistingDiscordGameState,
	)
}

// hostTalkVoiceStateMatches validates that delayed voice work still belongs to the same game and HostTalk revision.
// Phase and Running checks remain in the EM voice pipeline because they are normal AutoMute invariants.
func hostTalkVoiceStateMatches(current *GameState, expectedGuildID, expectedConnectCode string, expectedMode bool, expectedRevision uint64) bool {
	if current == nil {
		return false
	}
	return current.GuildID == expectedGuildID &&
		current.ConnectCode == expectedConnectCode &&
		current.GameStateMsg.HostTalkMode == expectedMode &&
		current.GameStateMsg.HostTalkRevision == expectedRevision
}

// hostTalkVoiceMember is a pure snapshot of the information required to plan one member.
// Discord lookup, actual mute-state comparison, sending, and persistence remain outside this layer.
// hostTalkVoiceRecord represents a voice state that is known to have been applied successfully,
// or was already observed in that exact state by Discord.
type hostTalkVoiceRecord struct {
	UserID                        string
	Mute                          bool
	Deaf                          bool
	ManagedAfterSuccess           bool
	ExpectedInTrackedVoiceChannel bool
}

// planHostTalkSuccessfulVoiceRecord creates a detached candidate state after successful voice application.
// It refuses to record stale work and never mutates current or its maps.
func planHostTalkSuccessfulVoiceRecord(
	current *GameState,
	expectedGuildID string,
	expectedConnectCode string,
	expectedPhase game.Phase,
	expectedMode bool,
	expectedRevision uint64,
	records []hostTalkVoiceRecord,
) (*GameState, bool, error) {
	if current == nil ||
		!current.Running ||
		current.GameData.GetPhase() != expectedPhase ||
		!hostTalkVoiceStateMatches(
			current,
			expectedGuildID,
			expectedConnectCode,
			expectedMode,
			expectedRevision,
		) {
		return current, false, errHostTalkVoiceSnapshotStale
	}

	candidate := *current
	candidate.GameStateMsg = current.GameStateMsg

	candidate.UserData = make(UserDataSet, len(current.UserData))
	for userID, userData := range current.UserData {
		candidate.UserData[userID] = userData
	}

	if current.GameStateMsg.HostTalkManagedUsers != nil {
		candidate.GameStateMsg.HostTalkManagedUsers = make(map[string]bool, len(current.GameStateMsg.HostTalkManagedUsers))
		for userID, managed := range current.GameStateMsg.HostTalkManagedUsers {
			candidate.GameStateMsg.HostTalkManagedUsers[userID] = managed
		}
	}

	changed := false
	for _, record := range records {
		if record.UserID == "" {
			continue
		}

		if userData, ok := candidate.UserData[record.UserID]; ok {
			if userData.ShouldBeMute != record.Mute || userData.ShouldBeDeaf != record.Deaf {
				userData.SetShouldBeMuteDeaf(record.Mute, record.Deaf)
				candidate.UserData[record.UserID] = userData
				changed = true
			}
		}

		if record.ManagedAfterSuccess {
			if candidate.GameStateMsg.HostTalkManagedUsers == nil {
				candidate.GameStateMsg.HostTalkManagedUsers = map[string]bool{}
			}
			if !candidate.GameStateMsg.HostTalkManagedUsers[record.UserID] {
				candidate.GameStateMsg.HostTalkManagedUsers[record.UserID] = true
				changed = true
			}
		} else if candidate.GameStateMsg.HostTalkManagedUsers != nil {
			if candidate.GameStateMsg.HostTalkManagedUsers[record.UserID] {
				delete(candidate.GameStateMsg.HostTalkManagedUsers, record.UserID)
				changed = true
			}
		}
	}

	return &candidate, changed, nil
}

// commitHostTalkSuccessfulVoiceRecord persists successful HostTalk bookkeeping before exposing it.
// Persistence failure returns the original state so callers never treat an unsaved candidate as committed.
func commitHostTalkSuccessfulVoiceRecord(
	current *GameState,
	expectedGuildID string,
	expectedConnectCode string,
	expectedPhase game.Phase,
	expectedMode bool,
	expectedRevision uint64,
	records []hostTalkVoiceRecord,
	persist func(*GameState) error,
) (*GameState, bool, error) {
	candidate, changed, err := planHostTalkSuccessfulVoiceRecord(
		current,
		expectedGuildID,
		expectedConnectCode,
		expectedPhase,
		expectedMode,
		expectedRevision,
		records,
	)
	if err != nil {
		return current, false, err
	}
	if !changed {
		return current, false, nil
	}
	if persist == nil {
		return current, false, errHostTalkPersistFailed
	}
	if err := persist(candidate); err != nil {
		return current, false, errors.Join(errHostTalkPersistFailed, err)
	}
	return candidate, true, nil
}

// hostTalkVoiceObservation is a current Discord voice/member snapshot.
// Unknown guild members are deliberately omitted so HostTalk never assumes an unknown account is human.
type hostTalkVoiceObservation struct {
	UserID                string
	IsBot                 bool
	InTrackedVoiceChannel bool
	Mute                  bool
	Deaf                  bool
}

// hostTalkPendingVoicePlan keeps the VC location that was validated immediately before a Discord change.
type hostTalkPendingVoicePlan struct {
	Plan                          hostTalkVoicePlan
	ExpectedInTrackedVoiceChannel bool
}

// observeHostTalkGuildVoiceMembers captures only voice users whose Discord member identity is known.
// This fail-closed behavior prevents accidentally muting a bot when member information is unavailable.
func observeHostTalkGuildVoiceMembers(guild *discordgo.Guild, trackedVoiceChannelID string) map[string]hostTalkVoiceObservation {
	return observeHostTalkGuildVoiceMembersWithResolver(guild, trackedVoiceChannelID, nil)
}

// observeHostTalkGuildVoiceMembersWithResolver permits the runtime adapter to resolve
// a voice user missing from the Guild member cache without enabling the GuildMembers intent.
// If both cache and resolver fail, the user remains excluded fail-closed.
func observeHostTalkGuildVoiceMembersWithResolver(
	guild *discordgo.Guild,
	trackedVoiceChannelID string,
	resolveMember func(string) (*discordgo.Member, error),
) map[string]hostTalkVoiceObservation {
	observations := map[string]hostTalkVoiceObservation{}
	if guild == nil {
		return observations
	}

	botByUserID := make(map[string]bool, len(guild.Members))
	for _, member := range guild.Members {
		if member == nil || member.User == nil || member.User.ID == "" {
			continue
		}
		botByUserID[member.User.ID] = member.User.Bot
	}

	for _, voiceState := range guild.VoiceStates {
		if voiceState == nil || voiceState.UserID == "" {
			continue
		}

		isBot, known := botByUserID[voiceState.UserID]
		if !known && resolveMember != nil {
			member, err := resolveMember(voiceState.UserID)
			if err == nil &&
				member != nil &&
				member.User != nil &&
				member.User.ID == voiceState.UserID {
				isBot = member.User.Bot
				known = true
			}
		}
		if !known {
			continue
		}

		observations[voiceState.UserID] = hostTalkVoiceObservation{
			UserID:                voiceState.UserID,
			IsBot:                 isBot,
			InTrackedVoiceChannel: voiceState.ChannelID != "" && voiceState.ChannelID == trackedVoiceChannelID,
			Mute:                  voiceState.Mute,
			Deaf:                  voiceState.Deaf,
		}
	}

	return observations
}

func hostTalkVoiceRecordForPlan(plan hostTalkVoicePlan, expectedInTrackedVoiceChannel bool) hostTalkVoiceRecord {
	return hostTalkVoiceRecord{
		UserID:                        plan.UserID,
		Mute:                          plan.Mute,
		Deaf:                          plan.Deaf,
		ManagedAfterSuccess:           plan.ManagedAfterSuccess,
		ExpectedInTrackedVoiceChannel: expectedInTrackedVoiceChannel,
	}
}

// partitionHostTalkVoicePlans separates states already observed on Discord from states requiring a request.
// Bots and users without a current known voice/member observation are skipped fail-closed.
func partitionHostTalkVoicePlans(
	plans []hostTalkVoicePlan,
	observations map[string]hostTalkVoiceObservation,
) ([]hostTalkVoiceRecord, []hostTalkPendingVoicePlan) {
	alreadyApplied := make([]hostTalkVoiceRecord, 0, len(plans))
	pending := make([]hostTalkPendingVoicePlan, 0, len(plans))

	for _, plan := range plans {
		observation, ok := observations[plan.UserID]
		if !ok || observation.IsBot {
			continue
		}

		if observation.Mute == plan.Mute && observation.Deaf == plan.Deaf {
			alreadyApplied = append(
				alreadyApplied,
				hostTalkVoiceRecordForPlan(plan, observation.InTrackedVoiceChannel),
			)
			continue
		}

		pending = append(pending, hostTalkPendingVoicePlan{
			Plan:                          plan,
			ExpectedInTrackedVoiceChannel: observation.InTrackedVoiceChannel,
		})
	}

	return alreadyApplied, pending
}

// filterHostTalkVoiceRecords revalidates bot status, voice presence, and tracked-VC location.
// For already-observed records, requireActualMatch additionally verifies mute/deaf still matches.
func filterHostTalkVoiceRecords(
	records []hostTalkVoiceRecord,
	observations map[string]hostTalkVoiceObservation,
	requireActualMatch bool,
) []hostTalkVoiceRecord {
	filtered := make([]hostTalkVoiceRecord, 0, len(records))
	for _, record := range records {
		observation, ok := observations[record.UserID]
		if !ok || observation.IsBot {
			continue
		}
		if observation.InTrackedVoiceChannel != record.ExpectedInTrackedVoiceChannel {
			continue
		}
		if requireActualMatch && (observation.Mute != record.Mute || observation.Deaf != record.Deaf) {
			continue
		}
		filtered = append(filtered, record)
	}
	return filtered
}

// filterHostTalkPendingVoicePlans is the last membership/bot gate immediately before Discord sending.
func filterHostTalkPendingVoicePlans(
	pending []hostTalkPendingVoicePlan,
	observations map[string]hostTalkVoiceObservation,
) []hostTalkPendingVoicePlan {
	filtered := make([]hostTalkPendingVoicePlan, 0, len(pending))
	for _, candidate := range pending {
		observation, ok := observations[candidate.Plan.UserID]
		if !ok || observation.IsBot {
			continue
		}
		if observation.InTrackedVoiceChannel != candidate.ExpectedInTrackedVoiceChannel {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return filtered
}

// recordsFromSuccessfulHostTalkPending prepares post-send bookkeeping only for users
// that are still human, still in voice, and still on the same tracked/outside side of the VC boundary.
func recordsFromSuccessfulHostTalkPending(
	pending []hostTalkPendingVoicePlan,
	observations map[string]hostTalkVoiceObservation,
) []hostTalkVoiceRecord {
	validated := filterHostTalkPendingVoicePlans(pending, observations)
	records := make([]hostTalkVoiceRecord, 0, len(validated))
	for _, candidate := range validated {
		records = append(
			records,
			hostTalkVoiceRecordForPlan(
				candidate.Plan,
				candidate.ExpectedInTrackedVoiceChannel,
			),
		)
	}
	return records
}

type hostTalkVoiceMember struct {
	UserID                string
	IsBot                 bool
	InTrackedVoiceChannel bool
	NormalApplicable      bool
	NormalMute            bool
	NormalDeaf            bool
	WasHostTalkManaged    bool
}

// hostTalkVoicePlan is the desired result for one member.
// ManagedAfterSuccess must only be persisted after the Discord operation succeeds.
type hostTalkVoicePlan struct {
	UserID              string
	Mute                bool
	Deaf                bool
	ManagedAfterSuccess bool
}

// planHostTalkVoiceBatch applies the same pure decision contract to a complete voice-member snapshot.
// Input order is preserved so the runtime adapter can retain the existing EM priority/order rules.
func planHostTalkVoiceBatch(hostTalkMode bool, leaderID string, members []hostTalkVoiceMember) []hostTalkVoicePlan {
	plans := make([]hostTalkVoicePlan, 0, len(members))
	for _, member := range members {
		decision := decideGameVoiceState(hostTalkVoiceInput{
			HostTalkMode:          hostTalkMode,
			LeaderID:              leaderID,
			UserID:                member.UserID,
			IsBot:                 member.IsBot,
			InTrackedVoiceChannel: member.InTrackedVoiceChannel,
			NormalApplicable:      member.NormalApplicable,
			NormalMute:            member.NormalMute,
			NormalDeaf:            member.NormalDeaf,
			WasHostTalkManaged:    member.WasHostTalkManaged,
		})
		if !decision.Apply {
			continue
		}
		plans = append(plans, hostTalkVoicePlan{
			UserID:              member.UserID,
			Mute:                decision.Mute,
			Deaf:                decision.Deaf,
			ManagedAfterSuccess: decision.ManagedAfterSuccess,
		})
	}
	return plans
}

type hostTalkVoiceInput struct {
	HostTalkMode          bool
	LeaderID              string
	UserID                string
	IsBot                 bool
	InTrackedVoiceChannel bool
	NormalApplicable      bool
	NormalMute            bool
	NormalDeaf            bool
	WasHostTalkManaged    bool
}

type hostTalkVoiceDecision struct {
	Apply               bool
	Mute                bool
	Deaf                bool
	ManagedAfterSuccess bool
}

// decideGameVoiceState only chooses a target; it neither sends nor records it.
// Priority: exclude bots, apply the host override, clean up managed users,
// apply normal rules, then do nothing. ManagedAfterSuccess is meaningful only
// when Apply is true and the caller has confirmed successful application.
func decideGameVoiceState(input hostTalkVoiceInput) hostTalkVoiceDecision {
	if input.IsBot {
		return hostTalkVoiceDecision{}
	}
	if input.HostTalkMode && input.InTrackedVoiceChannel && input.LeaderID != "" {
		return hostTalkVoiceDecision{
			Apply:               true,
			Mute:                input.UserID != input.LeaderID,
			Deaf:                false,
			ManagedAfterSuccess: true,
		}
	}
	if input.WasHostTalkManaged {
		return hostTalkVoiceDecision{
			Apply: true,
			Mute:  input.NormalApplicable && input.NormalMute,
			Deaf:  input.NormalApplicable && input.NormalDeaf,
		}
	}
	if input.NormalApplicable {
		return hostTalkVoiceDecision{
			Apply: true,
			Mute:  input.NormalMute,
			Deaf:  input.NormalDeaf,
		}
	}
	return hostTalkVoiceDecision{}
}
