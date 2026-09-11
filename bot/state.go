package bot

import "github.com/bwmarrin/discordgo"

// configureStateTracking adapts the upstream 8.5.0 cache optimization for EM.
// Keep guild, channel, thread, member, role, and voice state tracking enabled.
func configureStateTracking(s *discordgo.Session) {
	s.State.TrackEmojis = false
	s.State.TrackPresences = false
	s.AddHandler(trimUnusedGuildState)
}

// trimUnusedGuildState releases unused data from the initial GuildCreate payload.
// discordgo caches the guild before dispatching this handler.
func trimUnusedGuildState(s *discordgo.Session, m *discordgo.GuildCreate) {
	if s == nil || s.State == nil || !s.StateEnabled || m == nil || m.Guild == nil {
		return
	}
	g, err := s.State.Guild(m.ID)
	if err != nil || g == nil {
		return
	}

	s.State.Lock()
	defer s.State.Unlock()

	g.Emojis = nil
	g.Stickers = nil
	g.Presences = nil
	g.StageInstances = nil
}
