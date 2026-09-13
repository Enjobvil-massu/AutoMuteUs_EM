package bot

import (
	"errors"

	"github.com/bwmarrin/discordgo"
)

var errHostTalkRevisionOverflow = errors.New("host talk revision exhausted")

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
