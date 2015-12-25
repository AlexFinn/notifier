package mail

import (
	"crypto/tls"
	"fmt"
	"html/template"
	"io"
	"github.com/moira-alert/notifier"
	"strconv"
	"time"

	"github.com/gosexy/to"
	"github.com/op/go-logging"
	gomail "gopkg.in/gomail.v2"
)

var tpl = template.Must(template.New("mail").Parse(`
<html>
	<head>
		<style type="text/css">
			table { border-collapse: collapse; }
			table th, table td { padding: 0.5em; }
			tr.OK { background-color: #33cc99; color: white; }
			tr.WARN { background-color: #cccc32; color: white; }
			tr.ERROR { background-color: #cc0032; color: white; }
			tr.NODATA { background-color: #d3d3d3; color: black; }
			tr.EXCEPTION { background-color: #e14f4f; color: white; }
			th, td { border: 1px solid black; }
		</style>
	</head>
	<body>
		<table>
			<thead>
				<tr>
					<th>Timestamp</th>
					<th>Target</th>
					<th>Value</th>
					<th>Warn</th>
					<th>Error</th>
					<th>From</th>
					<th>To</th>
				</tr>
			</thead>
			<tbody>
				{{range .Items}}
				<tr class="{{ .State }}">
					<td>{{ .Timestamp }}</td>
					<td>{{ .Metric }}</td>
					<td>{{ .Value }}</td>
					<td>{{ .WarnValue }}</td>
					<td>{{ .ErrorValue }}</td>
					<td>{{ .Oldstate }}</td>
					<td>{{ .State }}</td>
				</tr>
				{{end}}
			</tbody>
		</table>
		<p><a href="{{ .Link }}">{{ .Link }}</a></p>
		{{if .Throttled}}
		<p>Please, <b>fix your system or tune this trigger</b> to generate less events.</p>
		{{end}}
	</body>
</html>
`))

var log *logging.Logger

type templateRow struct {
	Metric     string
	Timestamp  string
	Oldstate   string
	State      string
	Value      string
	WarnValue  string
	ErrorValue string
}

// Sender implements moira sender interface via pushover
type Sender struct {
	From        string
	SMTPhost    string
	SMTPport    int
	FrontURI    string
	InsecureTLS bool
}

// Init read yaml config
func (sender *Sender) Init(senderSettings map[string]string, logger *logging.Logger) error {
	sender.SetLogger(logger)
	sender.From = senderSettings["mail_from"]
	sender.SMTPhost = senderSettings["mail_smtp_host"]
	sender.SMTPport = int(to.Int64(senderSettings["mail_smtp_port"]))
	sender.InsecureTLS = to.Bool(senderSettings["mail_insecure_tls"])
	sender.FrontURI = senderSettings["front_uri"]
	return nil
}

// SetLogger for test purposes
func (sender *Sender) SetLogger(logger *logging.Logger) {
	log = logger
}

// MakeMessage prepare message to send
func (sender *Sender) MakeMessage(events []notifier.EventData, contact notifier.ContactData, trigger notifier.TriggerData, throttled bool) *gomail.Message {
	var subject string
	for _, tag := range trigger.Tags {
		subject = fmt.Sprintf("%s[%s]", subject, tag)
	}
	if len(events) == 1 {
		subject = fmt.Sprintf("%s %s", subject, events[0].State)
	} else {
		currentValue := make(map[string]int)
		for _, event := range events {
			currentValue[event.State]++
		}
		allStates := [...]string{"OK", "WARN", "ERROR", "NODATA", "TEST"}
		for _, state := range allStates {
			if currentValue[state] > 0 {
				subject = fmt.Sprintf("%s %s", subject, state)
			}
		}
	}

	subject = fmt.Sprintf("%s %s", subject, trigger.Name)

	templateData := struct {
		Link      string
		Throttled bool
		Items     []*templateRow
	}{
		Link:      fmt.Sprintf("%s/#/events/%s", sender.FrontURI, events[0].TriggerID),
		Throttled: throttled,
		Items:     make([]*templateRow, 0, len(events)),
	}

	for _, event := range events {
		templateData.Items = append(templateData.Items, &templateRow{
			Metric:     event.Metric,
			Timestamp:  time.Unix(event.Timestamp, 0).Format("15:04 02.01.2006"),
			Oldstate:   event.OldState,
			State:      event.State,
			Value:      strconv.FormatFloat(event.Value, 'f', -1, 64),
			WarnValue:  strconv.FormatFloat(trigger.WarnValue, 'f', -1, 64),
			ErrorValue: strconv.FormatFloat(trigger.ErrorValue, 'f', -1, 64),
		})
	}

	m := gomail.NewMessage()
	m.SetHeader("From", sender.From)
	m.SetHeader("To", contact.Value)
	m.SetHeader("Subject", subject)
	m.AddAlternativeWriter("text/html", func(w io.Writer) error {
		return tpl.Execute(w, templateData)
	})

	return m
}

//SendEvents implements Sender interface Send
func (sender *Sender) SendEvents(events []notifier.EventData, contact notifier.ContactData, trigger notifier.TriggerData, throttled bool) error {

	m := sender.MakeMessage(events, contact, trigger, throttled)

	d := gomail.Dialer{Host: sender.SMTPhost, Port: sender.SMTPport, TLSConfig: &tls.Config{InsecureSkipVerify: sender.InsecureTLS}}
	if err := d.DialAndSend(m); err != nil {
		return err
	}
	return nil
}
