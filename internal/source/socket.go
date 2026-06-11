package source

import (
	"context"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/kurenn/slack-tui/internal/data"
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
// Run reconnects internally on normal disconnects; if it ever returns (a fatal
// error), it's restarted with a backoff so live updates don't silently die.
// StopSocket tears everything down (workspace switching).
func (s *Slack) StartSocket(appToken, botToken string) {
	bot := slack.New(botToken, slack.OptionAppLevelToken(appToken))
	sm := socketmode.New(bot)
	s.events = make(chan Event, 128)
	ctx, cancel := context.WithCancel(context.Background())
	s.stopSocket = cancel

	go func() {
		defer close(s.events) // sole sender; a closed stream ends listenEvents loops
		for {
			var evt socketmode.Event
			var ok bool
			select {
			case <-ctx.Done():
				return
			case evt, ok = <-sm.Events:
				if !ok {
					return
				}
			}
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
			// Real new messages only: "" is a normal user message, "bot_message"
			// is what agents/bots post (exactly what users wait for). Everything
			// else (message_changed, message_deleted, channel_join, …) isn't a new
			// message and is skipped.
			if !ok || (me.SubType != "" && me.SubType != "bot_message") {
				continue
			}
			s.events <- Event{
				ConvID:   me.Channel,
				ThreadTS: me.ThreadTimeStamp,
				Msg: data.Message{
					ID: me.TimeStamp, UserID: socketAuthor(me), Time: tsTime(me.TimeStamp), Day: tsDay(me.TimeStamp),
					Text: s.renderText(me.Text), MentionsMe: s.mentionsMe(me.Text),
				},
			}
		}
	}()
	go func() {
		for {
			_ = sm.RunContext(ctx)
			if ctx.Err() != nil {
				return
			}
			time.Sleep(5 * time.Second)
		}
	}()
}

// StopSocket closes the Socket Mode connection and its event stream.
func (s *Slack) StopSocket() {
	if s.stopSocket != nil {
		s.stopSocket()
		s.stopSocket = nil
	}
}

// socketAuthor resolves who posted a Socket Mode message: bot_message events
// carry no user id, only a bot identity — prefer User, then Username, then a
// generic "bot" (mirrors messageAuthor for the Web API).
func socketAuthor(me *slackevents.MessageEvent) string {
	switch {
	case me.User != "":
		return me.User
	case me.Username != "":
		return me.Username
	case me.BotID != "":
		return "bot"
	}
	return me.User
}

// Events returns the real-time event stream, or nil if Socket Mode isn't running.
func (s *Slack) Events() <-chan Event { return s.events }
