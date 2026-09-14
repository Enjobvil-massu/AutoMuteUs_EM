package bot

import (
	"errors"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func hostTalkInteractionTestGame() *GameState {
	dgs := NewDiscordGameState("guild")
	dgs.ConnectCode = "ABCDEF"
	dgs.VoiceChannel = "tracked"
	dgs.GameStateMsg.MessageID = "message"
	dgs.GameStateMsg.MessageChannelID = "text"
	dgs.GameStateMsg.CreationTimeUnix = 1700000000
	dgs.GameStateMsg.LeaderID = "host"
	dgs.GameStateMsg.HostTalkRevision = 12
	return dgs
}

func hostTalkInteractionTestGuild() *discordgo.Guild {
	return &discordgo.Guild{
		ID:      "guild",
		OwnerID: "owner",
		VoiceStates: []*discordgo.VoiceState{
			{
				UserID:    "host",
				ChannelID: "tracked",
			},
			{
				UserID:    "participant",
				ChannelID: "tracked",
			},
		},
	}
}

func TestHostTalkInteractionPermissions(t *testing.T) {
	if got := hostTalkInteractionPermissions(nil); got != 0 {
		t.Fatalf(
			"nil interaction permissions = %d",
			got,
		)
	}

	i := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Member: &discordgo.Member{
				Permissions: discordgo.PermissionAdministrator |
					discordgo.PermissionViewChannel,
			},
		},
	}

	want := int64(
		discordgo.PermissionAdministrator |
			discordgo.PermissionViewChannel,
	)

	if got := hostTalkInteractionPermissions(i); got != want {
		t.Fatalf(
			"permissions = %d, want %d",
			got,
			want,
		)
	}
}

func TestHostTalkUsableInteractionGame(t *testing.T) {
	dgs := hostTalkInteractionTestGame()

	if !hostTalkUsableInteractionGame(dgs) {
		t.Fatal("valid interaction game rejected")
	}

	tests := []struct {
		name   string
		mutate func(*GameState)
	}{
		{
			name: "missing guild",
			mutate: func(g *GameState) {
				g.GuildID = ""
			},
		},
		{
			name: "missing connect code",
			mutate: func(g *GameState) {
				g.ConnectCode = ""
			},
		},
		{
			name: "missing message",
			mutate: func(g *GameState) {
				g.GameStateMsg.MessageID = ""
			},
		},
		{
			name: "missing creation time",
			mutate: func(g *GameState) {
				g.GameStateMsg.CreationTimeUnix = 0
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copy := *dgs
			copy.GameStateMsg = dgs.GameStateMsg

			tt.mutate(&copy)

			if hostTalkUsableInteractionGame(&copy) {
				t.Fatal("invalid interaction game accepted")
			}
		})
	}

	if hostTalkUsableInteractionGame(nil) {
		t.Fatal("nil interaction game accepted")
	}
}

func TestValidateHostTalkModeInteraction(t *testing.T) {
	base := hostTalkInteractionTestGame()
	guild := hostTalkInteractionTestGuild()

	currentControl, ok :=
		hostTalkControlFromState(
			base,
			true,
		)

	if !ok {
		t.Fatal("unable to create current control")
	}

	tests := []struct {
		name            string
		actor           string
		permissions     int64
		requestedMode   bool
		controlMutation func(*hostTalkControl)
		guildMutation   func(*discordgo.Guild)
		wantErr         error
	}{
		{
			name:          "host can enable",
			actor:         "host",
			requestedMode: true,
		},
		{
			name:          "owner can enable",
			actor:         "owner",
			requestedMode: true,
		},
		{
			name:          "administrator can enable",
			actor:         "admin",
			permissions:   discordgo.PermissionAdministrator,
			requestedMode: true,
		},
		{
			name:  "management permissions are not Administrator",
			actor: "configured-admin",
			permissions: discordgo.PermissionManageServer |
				discordgo.PermissionManageRoles |
				discordgo.PermissionVoiceMuteMembers,
			requestedMode: true,
			wantErr:       errHostTalkInteractionUnauthorized,
		},
		{
			name:          "normal participant denied",
			actor:         "participant",
			requestedMode: true,
			wantErr:       errHostTalkInteractionUnauthorized,
		},
		{
			name:          "stale revision denied",
			actor:         "host",
			requestedMode: true,
			controlMutation: func(c *hostTalkControl) {
				c.ExpectedRevision--
			},
			wantErr: errHostTalkInteractionStale,
		},
		{
			name:          "stale game generation denied",
			actor:         "host",
			requestedMode: true,
			controlMutation: func(c *hostTalkControl) {
				c.GameCreationTimeUnix--
			},
			wantErr: errHostTalkInteractionStale,
		},
		{
			name:          "wrong connect code denied",
			actor:         "host",
			requestedMode: true,
			controlMutation: func(c *hostTalkControl) {
				c.ConnectCode = "ZZZZZZ"
			},
			wantErr: errHostTalkInteractionStale,
		},
		{
			name:          "enable rejects host outside tracked VC",
			actor:         "host",
			requestedMode: true,
			guildMutation: func(g *discordgo.Guild) {
				g.VoiceStates = []*discordgo.VoiceState{
					{
						UserID:    "host",
						ChannelID: "other",
					},
				}
			},
			wantErr: errHostTalkInteractionHostNotInVC,
		},
		{
			name:          "disable allowed after host leaves VC",
			actor:         "host",
			requestedMode: false,
			guildMutation: func(g *discordgo.Guild) {
				g.VoiceStates = nil
			},
		},
		{
			name:          "administrator disables after host leaves VC",
			actor:         "admin",
			permissions:   discordgo.PermissionAdministrator,
			requestedMode: false,
			guildMutation: func(g *discordgo.Guild) {
				g.VoiceStates = nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dgs := *base
			dgs.GameStateMsg = base.GameStateMsg

			guildCopy := *guild
			guildCopy.VoiceStates =
				append(
					[]*discordgo.VoiceState(nil),
					guild.VoiceStates...,
				)

			control := currentControl
			control.RequestedMode =
				tt.requestedMode

			if tt.controlMutation != nil {
				tt.controlMutation(&control)
			}

			if tt.guildMutation != nil {
				tt.guildMutation(&guildCopy)
			}

			err := validateHostTalkModeInteraction(
				&dgs,
				&guildCopy,
				tt.actor,
				tt.permissions,
				tt.requestedMode,
				&control,
			)

			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf(
						"unexpected error: %v",
						err,
					)
				}
				return
			}

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf(
					"error = %v, want %v",
					err,
					tt.wantErr,
				)
			}
		})
	}
}

func TestHostTalkControlFromState(t *testing.T) {
	dgs := hostTalkInteractionTestGame()

	control, ok :=
		hostTalkControlFromState(
			dgs,
			true,
		)

	if !ok {
		t.Fatal("valid game did not create control")
	}

	if control.ConnectCode != dgs.ConnectCode ||
		control.GameCreationTimeUnix !=
			dgs.GameStateMsg.CreationTimeUnix ||
		control.ExpectedRevision !=
			dgs.GameStateMsg.HostTalkRevision ||
		!control.RequestedMode {
		t.Fatalf(
			"unexpected control: %#v",
			control,
		)
	}

	if _, ok :=
		hostTalkControlFromState(
			nil,
			true,
		); ok {
		t.Fatal("nil game created control")
	}
}

func TestHostTalkActionNotice(t *testing.T) {
	changed :=
		hostTalkActionNotice(
			true,
			true,
			true,
		)

	if !strings.Contains(changed, "**ON**") ||
		!strings.Contains(changed, "にしました") {
		t.Fatalf(
			"unexpected changed notice: %q",
			changed,
		)
	}

	idempotent :=
		hostTalkActionNotice(
			false,
			false,
			false,
		)

	if !strings.Contains(idempotent, "**OFF**") ||
		!strings.Contains(idempotent, "再同期") {
		t.Fatalf(
			"unexpected idempotent notice: %q",
			idempotent,
		)
	}

	concurrent :=
		hostTalkActionNotice(
			true,
			true,
			false,
		)

	if !strings.Contains(concurrent, "別の操作") ||
		!strings.Contains(concurrent, "**OFF**") {
		t.Fatalf(
			"unexpected concurrent notice: %q",
			concurrent,
		)
	}
}

func TestHostTalkSelectorResponse(t *testing.T) {
	dgs := hostTalkInteractionTestGame()
	dgs.GameStateMsg.HostTalkMode = true

	initial, err :=
		hostTalkSelectorResponse(
			dgs,
			false,
			"",
		)

	if err != nil {
		t.Fatal(err)
	}

	if initial.Type !=
		discordgo.InteractionResponseChannelMessageWithSource {
		t.Fatalf(
			"initial type = %v",
			initial.Type,
		)
	}

	if initial.Data == nil ||
		initial.Data.Flags&(1<<6) == 0 {
		t.Fatal("initial selector is not ephemeral")
	}

	if len(initial.Data.Components) != 1 ||
		!strings.Contains(
			initial.Data.Content,
			"**ON**",
		) {
		t.Fatalf(
			"unexpected initial selector: %#v",
			initial.Data,
		)
	}

	update, err :=
		hostTalkSelectorResponse(
			dgs,
			true,
			"✅ updated",
		)

	if err != nil {
		t.Fatal(err)
	}

	if update.Type !=
		discordgo.InteractionResponseUpdateMessage {
		t.Fatalf(
			"update type = %v",
			update.Type,
		)
	}

	if update.Data == nil ||
		!strings.Contains(
			update.Data.Content,
			"✅ updated",
		) ||
		!strings.Contains(
			update.Data.Content,
			"🎙️ ホスト発言モード",
		) {
		t.Fatalf(
			"unexpected updated selector: %#v",
			update.Data,
		)
	}
}
