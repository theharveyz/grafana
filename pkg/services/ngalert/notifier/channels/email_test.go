package channels

import (
	"context"
	"net/url"
	"testing"

	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/bus"
	"github.com/grafana/grafana/pkg/components/simplejson"
	"github.com/grafana/grafana/pkg/models"
	"github.com/grafana/grafana/pkg/services/notifications"
	"github.com/grafana/grafana/pkg/setting"
)

func TestEmailNotifier(t *testing.T) {
	tmpl := templateForTests(t)

	externalURL, err := url.Parse("http://localhost/base")
	require.NoError(t, err)
	tmpl.ExternalURL = externalURL

	t.Run("empty settings should return error", func(t *testing.T) {
		json := `{ }`

		settingsJSON, _ := simplejson.NewJson([]byte(json))
		model := &NotificationChannelConfig{
			Name:     "ops",
			Type:     "email",
			Settings: settingsJSON,
		}

		_, err := NewEmailNotifier(model, tmpl)
		require.Error(t, err)
	})

	t.Run("with the correct settings it should not fail and produce the expected command", func(t *testing.T) {
		json := `{
			"addresses": "someops@example.com;somedev@example.com",
			"message": "{{ template \"default.title\" . }}"
		}`
		settingsJSON, err := simplejson.NewJson([]byte(json))
		require.NoError(t, err)

		emailNotifier, err := NewEmailNotifier(&NotificationChannelConfig{
			Name:     "ops",
			Type:     "email",
			Settings: settingsJSON,
		}, tmpl)

		require.NoError(t, err)

		expected := map[string]interface{}{}
		bus.AddHandlerCtx("test", func(ctx context.Context, cmd *models.SendEmailCommand) error {
			expected["subject"] = cmd.Subject
			expected["to"] = cmd.To
			expected["single_email"] = cmd.SingleEmail
			expected["template"] = cmd.Template
			expected["data"] = cmd.Data

			return nil
		})

		alerts := []*types.Alert{
			{
				Alert: model.Alert{
					Labels:      model.LabelSet{"alertname": "AlwaysFiring", "severity": "warning"},
					Annotations: model.LabelSet{"runbook_url": "http://fix.me", "__dashboardUid__": "abc", "__panelId__": "5"},
				},
			},
		}

		ok, err := emailNotifier.Notify(context.Background(), alerts...)
		require.NoError(t, err)
		require.True(t, ok)

		require.Equal(t, map[string]interface{}{
			"subject":      "[FIRING:1]  (AlwaysFiring warning)",
			"to":           []string{"someops@example.com", "somedev@example.com"},
			"single_email": false,
			"template":     "ng_alert_notification",
			"data": map[string]interface{}{
				"Title":   "[FIRING:1]  (AlwaysFiring warning)",
				"Message": "[FIRING:1]  (AlwaysFiring warning)",
				"Status":  "firing",
				"Alerts": ExtendedAlerts{
					ExtendedAlert{
						Status:       "firing",
						Labels:       template.KV{"alertname": "AlwaysFiring", "severity": "warning"},
						Annotations:  template.KV{"runbook_url": "http://fix.me"},
						Fingerprint:  "15a37193dce72bab",
						SilenceURL:   "http://localhost/base/alerting/silence/new?alertmanager=grafana&matchers=alertname%3DAlwaysFiring%2Cseverity%3Dwarning",
						DashboardURL: "http://localhost/base/d/abc",
						PanelURL:     "http://localhost/base/d/abc?viewPanel=5",
					},
				},
				"GroupLabels":       template.KV{},
				"CommonLabels":      template.KV{"alertname": "AlwaysFiring", "severity": "warning"},
				"CommonAnnotations": template.KV{"runbook_url": "http://fix.me"},
				"ExternalURL":       "http://localhost/base",
				"RuleUrl":           "http://localhost/base/alerting/list",
				"AlertPageUrl":      "http://localhost/base/alerting/list?alertState=firing&view=state",
			},
		}, expected)
	})
}

func TestEmailNotifierIntegration(t *testing.T) {
	ns, bus := createCoreEmailService(t)

	emailTmpl := templateForTests(t)
	externalURL, err := url.Parse("http://localhost/base")
	require.NoError(t, err)
	emailTmpl.ExternalURL = externalURL

	cases := []struct {
		name        string
		alerts      []*types.Alert
		messageTmpl string
		expSubject  string
		expSnippets []string
	}{
		{
			name: "single alert with templated message",
			alerts: []*types.Alert{
				{
					Alert: model.Alert{
						Labels:      model.LabelSet{"alertname": "AlwaysFiring", "severity": "warning"},
						Annotations: model.LabelSet{"runbook_url": "http://fix.me", "__dashboardUid__": "abc", "__panelId__": "5"},
					},
				},
			},
			messageTmpl: `Hi, this is a custom template.
				{{ if gt (len .Alerts.Firing) 0 }}
					You have {{ len .Alerts.Firing }} alerts firing.
					{{ range .Alerts.Firing }} Firing: {{ .Labels.alertname }} at {{ .Labels.severity }} {{ end }}
				{{ end }}`,
			expSubject: "[FIRING:1]  (AlwaysFiring warning)",
			expSnippets: []string{
				"Hi, this is a custom template.",
				"You have 1 alerts firing.",
				"Firing: AlwaysFiring at warning",
			},
		},
		{
			name: "multiple alerts with templated message",
			alerts: []*types.Alert{
				{
					Alert: model.Alert{
						Labels:      model.LabelSet{"alertname": "FiringOne", "severity": "warning"},
						Annotations: model.LabelSet{"runbook_url": "http://fix.me", "__dashboardUid__": "abc", "__panelId__": "5"},
					},
				},
				{
					Alert: model.Alert{
						Labels:      model.LabelSet{"alertname": "FiringTwo", "severity": "critical"},
						Annotations: model.LabelSet{"runbook_url": "http://fix.me", "__dashboardUid__": "abc", "__panelId__": "5"},
					},
				},
			},
			messageTmpl: `Hi, this is a custom template.
				{{ if gt (len .Alerts.Firing) 0 }}
					You have {{ len .Alerts.Firing }} alerts firing.
					{{ range .Alerts.Firing }} Firing: {{ .Labels.alertname }} at {{ .Labels.severity }} {{ end }}
				{{ end }}`,
			expSubject: "[FIRING:2]  ",
			expSnippets: []string{
				"Hi, this is a custom template.",
				"You have 2 alerts firing.",
				"Firing: FiringOne at warning",
				"Firing: FiringTwo at critical",
			},
		},
		{
			name: "empty message with alerts uses default template content",
			alerts: []*types.Alert{
				{
					Alert: model.Alert{
						Labels:      model.LabelSet{"alertname": "FiringOne", "severity": "warning"},
						Annotations: model.LabelSet{"runbook_url": "http://fix.me", "__dashboardUid__": "abc", "__panelId__": "5"},
					},
				},
				{
					Alert: model.Alert{
						Labels:      model.LabelSet{"alertname": "FiringTwo", "severity": "critical"},
						Annotations: model.LabelSet{"runbook_url": "http://fix.me", "__dashboardUid__": "abc", "__panelId__": "5"},
					},
				},
			},
			messageTmpl: "",
			expSubject:  "[FIRING:2]  ",
			expSnippets: []string{
				"Firing: 2 alerts",
				"<li>alertname: FiringOne</li><li>severity: warning</li>",
				"<li>alertname: FiringTwo</li><li>severity: critical</li>",
				"<a href=\"http://fix.me\"",
				"<a href=\"http://localhost/base/d/abc",
				"<a href=\"http://localhost/base/d/abc?viewPanel=5",
			},
		},
		{
			name: "message containing HTML gets HTMLencoded",
			alerts: []*types.Alert{
				{
					Alert: model.Alert{
						Labels:      model.LabelSet{"alertname": "AlwaysFiring", "severity": "warning"},
						Annotations: model.LabelSet{"runbook_url": "http://fix.me", "__dashboardUid__": "abc", "__panelId__": "5"},
					},
				},
			},
			messageTmpl: `<marquee>Hi, this is a custom template.</marquee>
				{{ if gt (len .Alerts.Firing) 0 }}
					<ol>
					{{range .Alerts.Firing }}<li>Firing: {{ .Labels.alertname }} at {{ .Labels.severity }} </li> {{ end }}
					</ol>
				{{ end }}`,
			expSubject: "[FIRING:1]  (AlwaysFiring warning)",
			expSnippets: []string{
				"&lt;marquee&gt;Hi, this is a custom template.&lt;/marquee&gt;",
				"&lt;li&gt;Firing: AlwaysFiring at warning &lt;/li&gt;",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			emailNotifier := createSut(t, c.messageTmpl, emailTmpl, bus)

			ok, err := emailNotifier.Notify(context.Background(), c.alerts...)
			require.NoError(t, err)
			require.True(t, ok)

			sentMsg := ns.MailQueuePop()

			require.NotNil(t, sentMsg)

			require.Equal(t, "\"Grafana Admin\" <from@address.com>", sentMsg.From)
			require.Equal(t, sentMsg.To[0], "someops@example.com")

			require.Equal(t, c.expSubject, sentMsg.Subject)

			require.Contains(t, sentMsg.Body, "text/html")
			html := sentMsg.Body["text/html"]
			require.NotNil(t, html)

			for _, s := range c.expSnippets {
				require.Contains(t, html, s)
			}
		})
	}
}

func createCoreEmailService(t *testing.T) (*notifications.NotificationService, *bus.InProcBus) {
	t.Helper()

	setting.StaticRootPath = "../../../public/"
	setting.BuildVersion = "4.0.0"

	ns := &notifications.NotificationService{}
	bus := bus.New()
	cfg := setting.NewCfg()
	cfg.Smtp.Enabled = true
	cfg.Smtp.TemplatesPatterns = []string{"/home/alexweav/git/grafana/public/emails/*.html", "/home/alexweav/git/grafana/public/emails/*.txt"}
	cfg.Smtp.FromAddress = "from@address.com"
	cfg.Smtp.FromName = "Grafana Admin"
	cfg.Smtp.ContentTypes = []string{"text/html", "text/plain"}
	cfg.Smtp.Host = "localhost:1234"

	ns, err := notifications.ProvideService(bus, cfg)
	require.NoError(t, err)

	return ns, bus
}

func createSut(t *testing.T, messageTmpl string, emailTmpl *template.Template, bus bus.Bus) *EmailNotifier {
	t.Helper()

	json := `{
		"addresses": "someops@example.com;somedev@example.com"
	}`
	settingsJSON, err := simplejson.NewJson([]byte(json))
	if messageTmpl != "" {
		settingsJSON.Set("message", messageTmpl)
	}
	require.NoError(t, err)

	emailNotifier, err := NewEmailNotifier(&NotificationChannelConfig{
		Name:     "ops",
		Type:     "email",
		Settings: settingsJSON,
	}, emailTmpl)
	require.NoError(t, err)
	emailNotifier.bus = bus

	return emailNotifier
}
