package source

import (
	"strings"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/abrahamkuri/slack-tui/internal/data"
)

// Event is a real-time message delivered over Socket Mode.
type Event struct {
	ConvID   string
	Msg      data.Message
	ThreadTS string // set when the message is a thread reply
}

// StartSocket opens a Socket Mode connection (app-level token + bot token) and
// streams incoming message events on Events(). It only receives events for
// channels/DMs the app is a member of — that's a Slack platform constraint.
func (s *Slack) StartSocket(appToken, botToken string) {
	bot := slack.New(botToken, slack.OptionAppLevelToken(appToken))
	sm := socketmode.New(bot)
	s.events = make(chan Event, 128)

	go func() {
		for evt := range sm.Events {
			if evt.Type != socketmode.EventTypeEventsAPI {
				continue
			}
			if evt.Request != nil {
				sm.Ack(*evt.Request)
			}
			eapi, ok := evt.Data.(slackevents.EventsAPIEvent)
			if !ok {
				continue
			}
			me, ok := eapi.InnerEvent.Data.(*slackevents.MessageEvent)
			if !ok || me.SubType != "" { // only ordinary new messages (skip joins/edits/etc.)
				continue
			}
			s.events <- Event{
				ConvID:   me.Channel,
				ThreadTS: me.ThreadTimeStamp,
				Msg: data.Message{
					ID: me.TimeStamp, UserID: me.User, Time: tsTime(me.TimeStamp),
					Text: s.renderText(me.Text), MentionsMe: strings.Contains(me.Text, "<@"+s.meID+">"),
				},
			}
		}
	}()
	go func() { _ = sm.Run() }()
}

// Events returns the real-time event stream, or nil if Socket Mode isn't running.
func (s *Slack) Events() <-chan Event { return s.events }
