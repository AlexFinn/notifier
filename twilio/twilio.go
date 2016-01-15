package twilio

import (
	"fmt"
	"strconv"
	"time"
	"bytes"

	"github.com/op/go-logging"

	"github.com/moira-alert/notifier"

	twilio "github.com/carlosdp/twiliogo"
)

type sendEventsTwilio interface {
	SendEvents(events notifier.EventsData, contact notifier.ContactData, trigger notifier.TriggerData, throttled bool) error
}

type twilioSender struct {
	client       *twilio.TwilioClient
	APIFromPhone string
	log          *logging.Logger
}

type twilioSenderSms struct {
	twilioSender
}

type twilioSenderVoice struct {
	twilioSender
}

func (smsSender *twilioSenderSms) SendEvents(events notifier.EventsData, contact notifier.ContactData, trigger notifier.TriggerData, throttled bool) error {
	var message bytes.Buffer

	state := events.GetSubjectState()
	tags := trigger.GetTags()

	message.WriteString(fmt.Sprintf("%s %s %s\n\n", state, trigger.Name, tags))

	for _, event := range events {
		value := strconv.FormatFloat(event.Value, 'f', -1, 64)
		message.WriteString(fmt.Sprintf("%s: %s = %s (%s to %s)\n", time.Unix(event.Timestamp, 0).Format("15:04"), event.Metric, value, event.OldState, event.State))
	}

	if len(events) > 5 {
		message.WriteString(fmt.Sprintf("\n...and %d more events.", len(events)-5))
	}

	if throttled {
		message.WriteString("\nPlease, fix your system or tune this trigger to generate less events.")
	}

	smsSender.log.Debug("Calling twilio sms api to phone %s and message body %s", contact.Value, message.String())
	twilioMessage, err := twilio.NewMessage(smsSender.client, smsSender.APIFromPhone, contact.Value, twilio.Body(message.String()))

	if err != nil {
		return fmt.Errorf("Failed to send message to contact %s: %s", contact.Value, err.Error())
	}

	smsSender.log.Debug(fmt.Sprintf("message send to twilio with status: %s", twilioMessage.Status))

	return nil
}

func (voiceSender *twilioSenderVoice) SendEvents(events notifier.EventsData, contact notifier.ContactData, trigger notifier.TriggerData, throttled bool) error {
	twilio.NewCall(voiceSender.client, voiceSender.APIFromPhone, contact.Value, nil)
	return nil
}

// Sender implements moira sender interface via twilio
type Sender struct {
	sender sendEventsTwilio
}

//Init read yaml config
func (sender *Sender) Init(senderSettings map[string]string, logger *logging.Logger) error {
	apiSID := senderSettings["api_sid"]
	if apiSID == "" {
		return fmt.Errorf("Can not read twilio api_sid from config")
	}

	apiSecret := senderSettings["api_secret"]
	if apiSecret == "" {
		return fmt.Errorf("Can not read twilio api_secret from config")
	}

	apiFromPhone := senderSettings["api_fromphone"]
	if apiFromPhone == "" {
		return fmt.Errorf("Can not read twilio from phone")
	}

	apiType := senderSettings["type"]
	twilioClient := twilio.NewClient(apiSID, apiSecret)

	switch apiType {
	case "twilio sms":
		sender.sender = &twilioSenderSms{twilioSender{twilioClient, apiFromPhone, logger}}

	case "twilio voice":
		sender.sender = &twilioSenderVoice{twilioSender{twilioClient, apiFromPhone, logger}}

	default:
		return fmt.Errorf("Wrong twilio type: %s", apiType)
	}

	return nil
}

//SendEvents implements Sender interface Send
func (sender *Sender) SendEvents(events notifier.EventsData, contact notifier.ContactData, trigger notifier.TriggerData, throttled bool) error {
	return sender.sender.SendEvents(events, contact, trigger, throttled)
}
