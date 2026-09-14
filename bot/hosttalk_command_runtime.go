package bot

import (
	"errors"
	"fmt"

	"github.com/automuteus/automuteus/v8/bot/command"
	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/bwmarrin/discordgo"
)

var (
	errHostTalkInteractionUnauthorized = errors.New("HostTalk interaction unauthorized")
	errHostTalkInteractionHostNotInVC  = errors.New("HostTalk host not in tracked voice channel")
	errHostTalkInteractionStale        = errors.New("HostTalk interaction stale")
)

func hostTalkInteractionPermissions(i *discordgo.InteractionCreate) int64 {
	if i == nil || i.Interaction == nil || i.Member == nil {
		return 0
	}
	return i.Member.Permissions
}

func hostTalkUsableInteractionGame(dgs *GameState) bool {
	return dgs != nil &&
		dgs.GuildID != "" &&
		dgs.ConnectCode != "" &&
		dgs.GameStateMsg.Exists() &&
		dgs.GameStateMsg.CreationTimeUnix > 0
}

func hostTalkControlFromState(
	dgs *GameState,
	requestedMode bool,
) (hostTalkControl, bool) {
	if !hostTalkUsableInteractionGame(dgs) {
		return hostTalkControl{}, false
	}

	return hostTalkControl{
		ConnectCode:          dgs.ConnectCode,
		GameCreationTimeUnix: dgs.GameStateMsg.CreationTimeUnix,
		ExpectedRevision:     dgs.GameStateMsg.HostTalkRevision,
		RequestedMode:        requestedMode,
	}, true
}

// validateHostTalkModeInteraction is the common action-time validation used by
// direct /hostmute mode:<on|off> execution and revision-bound buttons.
func validateHostTalkModeInteraction(
	current *GameState,
	guild *discordgo.Guild,
	actorUserID string,
	actorPermissions int64,
	requestedMode bool,
	expectedControl *hostTalkControl,
) error {
	if !hostTalkUsableInteractionGame(current) {
		return errHostTalkGameStateUnavailable
	}

	if expectedControl != nil &&
		!hostTalkControlMatchesGame(
			current,
			*expectedControl,
		) {
		return errHostTalkInteractionStale
	}

	guildOwnerID := ""
	if guild != nil {
		guildOwnerID = guild.OwnerID
	}

	if !isHostTalkAuthorized(
		current.GameStateMsg.LeaderID,
		actorUserID,
		guildOwnerID,
		actorPermissions,
	) {
		return errHostTalkInteractionUnauthorized
	}

	// Only ON requires the current host to be in the tracked VC.
	// OFF must remain possible after the host leaves.
	if requestedMode &&
		!hostTalkEnablePreflight(
			guild,
			current.GameStateMsg.LeaderID,
			current.VoiceChannel,
		) {
		return errHostTalkInteractionHostNotInVC
	}

	return nil
}

// commitHostTalkInteractionMode serializes validation and mode transition
// with the existing Redis GameState lock.
//
// staleIfGameMissing is true for an old selector button and false for a fresh
// slash command. A vanished selector target is stale; lock contention remains
// a real lock error.
func (bot *Bot) commitHostTalkInteractionMode(
	guild *discordgo.Guild,
	guildID string,
	actorUserID string,
	actorPermissions int64,
	control hostTalkControl,
	staleIfGameMissing bool,
) (*GameState, bool, error) {
	if bot == nil || bot.RedisInterface == nil {
		return nil, false, errHostTalkGameStateUnavailable
	}

	gsr := GameStateRequest{
		GuildID:     guildID,
		ConnectCode: control.ConnectCode,
	}

	if bot.RedisInterface.getDiscordGameStateKey(gsr) == "" {
		if staleIfGameMissing {
			return nil, false, errHostTalkInteractionStale
		}
		return nil, false, errHostTalkGameStateUnavailable
	}

	lock, current :=
		bot.RedisInterface.getExistingDiscordGameStateAndLock(
			gsr,
		)

	if lock == nil {
		return nil, false, errHostTalkLockUnavailable
	}

	defer lock.Release(ctx)

	if current == nil {
		if staleIfGameMissing {
			return nil, false, errHostTalkInteractionStale
		}
		return nil, false, errHostTalkGameStateUnavailable
	}

	if err := validateHostTalkModeInteraction(
		current,
		guild,
		actorUserID,
		actorPermissions,
		control.RequestedMode,
		&control,
	); err != nil {
		return current, false, err
	}

	return commitHostTalkGameState(
		current,
		control.RequestedMode,
		bot.RedisInterface.persistExistingDiscordGameState,
	)
}

func hostTalkActionNotice(
	requestedMode bool,
	changed bool,
	currentMode bool,
) string {
	if currentMode != requestedMode {
		return fmt.Sprintf(
			"ℹ️ 別の操作で状態が更新されました。現在のホスト発言モードは **%s** です。",
			hostTalkModeText(currentMode),
		)
	}

	if changed {
		return fmt.Sprintf(
			"✅ 🎙️ ホスト発言モードを **%s** にしました。",
			hostTalkModeText(currentMode),
		)
	}

	return fmt.Sprintf(
		"ℹ️ 🎙️ ホスト発言モードはすでに **%s** です。現在の音声状態を再同期しました。",
		hostTalkModeText(currentMode),
	)
}

func hostTalkSelectorResponse(
	dgs *GameState,
	updateExistingMessage bool,
	notice string,
) (*discordgo.InteractionResponse, error) {
	if !hostTalkUsableInteractionGame(dgs) {
		return nil, errHostTalkGameStateUnavailable
	}

	components, err :=
		hostTalkSelectorComponents(
			dgs.ConnectCode,
			dgs.GameStateMsg.CreationTimeUnix,
			dgs.GameStateMsg.HostTalkRevision,
		)

	if err != nil {
		return nil, err
	}

	content :=
		hostTalkSelectorContent(
			dgs.GameStateMsg.HostTalkMode,
		)

	if notice != "" {
		content = notice + "\n\n" + content
	}

	if updateExistingMessage {
		return &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    content,
				Components: components,
			},
		}, nil
	}

	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:      1 << 6,
			Content:    content,
			Components: components,
		},
	}, nil
}

func hostTalkInteractionErrorResponse(
	err error,
	sett *settings.GuildSettings,
) *discordgo.InteractionResponse {
	switch {
	case errors.Is(
		err,
		errHostTalkInteractionUnauthorized,
	):
		return command.PrivateResponse(
			hostTalkUnauthorizedMessage,
		)

	case errors.Is(
		err,
		errHostTalkInteractionHostNotInVC,
	):
		return command.PrivateResponse(
			hostTalkHostNotInVCMessage,
		)

	case errors.Is(
		err,
		errHostTalkInteractionStale,
	):
		return command.PrivateResponse(
			hostTalkStaleControlMessage,
		)

	case errors.Is(
		err,
		errHostTalkGameStateUnavailable,
	):
		return command.NoGameResponse(sett)

	case errors.Is(
		err,
		errHostTalkLockUnavailable,
	):
		return command.DeadlockGameStateResponse(
			command.HostMute.Name,
			sett,
		)

	default:
		return command.PrivateErrorResponse(
			command.HostMute.Name,
			err,
			sett,
		)
	}
}

func (bot *Bot) reconcileHostTalkInteractionNow(
	s *discordgo.Session,
	sett *settings.GuildSettings,
	dgs *GameState,
) {
	if bot == nil ||
		s == nil ||
		sett == nil ||
		dgs == nil ||
		dgs.GuildID == "" ||
		dgs.ConnectCode == "" {
		return
	}

	// Reuse the exact existing normal AutoMute + HostTalk planner/runtime.
	// delay=0 makes the explicit action immediate without introducing a second
	// voice-state implementation.
	bot.handleTrackedMembers(
		s,
		sett,
		0,
		NoPriority,
		GameStateRequest{
			GuildID:     dgs.GuildID,
			ConnectCode: dgs.ConnectCode,
		},
	)
}

func (bot *Bot) handleHostTalkSlashCommand(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	guild *discordgo.Guild,
	sett *settings.GuildSettings,
	gsr GameStateRequest,
) *discordgo.InteractionResponse {
	if bot == nil ||
		bot.RedisInterface == nil ||
		i == nil ||
		i.Interaction == nil ||
		i.Member == nil ||
		i.Member.User == nil {
		return command.NoGameResponse(sett)
	}

	current :=
		bot.RedisInterface.GetReadOnlyDiscordGameState(
			gsr,
		)

	if !hostTalkUsableInteractionGame(current) {
		return command.NoGameResponse(sett)
	}

	actorUserID := i.Member.User.ID
	permissions :=
		hostTalkInteractionPermissions(i)

	modeValue, modeProvided :=
		command.GetHostMuteMode(
			i.ApplicationCommandData().Options,
		)

	// /hostmute without mode only displays the private selector.
	// No state mutation occurs here. Every actual button action is validated
	// again under the current GameState lock.
	if !modeProvided {
		guildOwnerID := ""
		if guild != nil {
			guildOwnerID = guild.OwnerID
		}

		if !isHostTalkAuthorized(
			current.GameStateMsg.LeaderID,
			actorUserID,
			guildOwnerID,
			permissions,
		) {
			return command.PrivateResponse(
				hostTalkUnauthorizedMessage,
			)
		}

		resp, err :=
			hostTalkSelectorResponse(
				current,
				false,
				"",
			)

		if err != nil {
			return hostTalkInteractionErrorResponse(
				err,
				sett,
			)
		}

		return resp
	}

	requestedMode, valid :=
		command.ParseHostMuteMode(modeValue)

	if !valid {
		return command.PrivateResponse(
			"ホスト発言モードは ON または OFF を指定してください。",
		)
	}

	control, ok :=
		hostTalkControlFromState(
			current,
			requestedMode,
		)

	if !ok {
		return command.NoGameResponse(sett)
	}

	updated, changed, err :=
		bot.commitHostTalkInteractionMode(
			guild,
			i.GuildID,
			actorUserID,
			permissions,
			control,
			false,
		)

	if err != nil {
		return hostTalkInteractionErrorResponse(
			err,
			sett,
		)
	}

	// Idempotent ON/ON and OFF/OFF also reconcile so a previous partial
	// Discord failure can be safely retried without incrementing revision.
	bot.reconcileHostTalkInteractionNow(
		s,
		sett,
		updated,
	)

	latest :=
		bot.RedisInterface.GetReadOnlyDiscordGameState(
			GameStateRequest{
				GuildID:     i.GuildID,
				ConnectCode: control.ConnectCode,
			},
		)

	if latest == nil {
		return command.NoGameResponse(sett)
	}

	return command.PrivateResponse(
		hostTalkActionNotice(
			requestedMode,
			changed,
			latest.GameStateMsg.HostTalkMode,
		),
	)
}

func (bot *Bot) handleHostTalkComponent(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	guild *discordgo.Guild,
	sett *settings.GuildSettings,
	customID string,
) *discordgo.InteractionResponse {
	if bot == nil ||
		bot.RedisInterface == nil ||
		i == nil ||
		i.Interaction == nil ||
		i.Member == nil ||
		i.Member.User == nil {
		return command.PrivateResponse(
			hostTalkStaleControlMessage,
		)
	}

	control, ok :=
		parseHostTalkControlID(customID)

	if !ok {
		return command.PrivateResponse(
			hostTalkStaleControlMessage,
		)
	}

	updated, changed, err :=
		bot.commitHostTalkInteractionMode(
			guild,
			i.GuildID,
			i.Member.User.ID,
			hostTalkInteractionPermissions(i),
			control,
			true,
		)

	if err != nil {
		return hostTalkInteractionErrorResponse(
			err,
			sett,
		)
	}

	bot.reconcileHostTalkInteractionNow(
		s,
		sett,
		updated,
	)

	latest :=
		bot.RedisInterface.GetReadOnlyDiscordGameState(
			GameStateRequest{
				GuildID:     i.GuildID,
				ConnectCode: control.ConnectCode,
			},
		)

	if latest == nil ||
		!hostTalkUsableInteractionGame(latest) {
		return command.PrivateResponse(
			hostTalkStaleControlMessage,
		)
	}

	notice :=
		hostTalkActionNotice(
			control.RequestedMode,
			changed,
			latest.GameStateMsg.HostTalkMode,
		)

	resp, err :=
		hostTalkSelectorResponse(
			latest,
			true,
			notice,
		)

	if err != nil {
		return hostTalkInteractionErrorResponse(
			err,
			sett,
		)
	}

	return resp
}
