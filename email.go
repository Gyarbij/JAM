package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"html/template"
	"net/smtp"
	"strings"
	"time"

	jEmail "github.com/jordan-wright/email"
	"github.com/knz/strtime"
	"github.com/mailgun/mailgun-go/v4"
)

type Emailer struct {
	smtpAuth                                 smtp.Auth
	sendType, sendMethod, fromAddr, fromName string
	content                                  Email
	mg                                       *mailgun.MailgunImpl
	mime                                     string
	host                                     string
}

type Email struct {
	subject    string
	html, text string
}

func (email *Emailer) formatExpiry(expiry time.Time, tzaware bool, datePattern, timePattern string) (d, t, expires_in string) {
	d, _ = strtime.Strftime(expiry, datePattern)
	t, _ = strtime.Strftime(expiry, timePattern)
	current_time := time.Now()
	if tzaware {
		current_time = current_time.UTC()
	}
	_, _, days, hours, minutes, _ := timeDiff(expiry, current_time)
	if days != 0 {
		expires_in += fmt.Sprintf("%dd ", days)
	}
	if hours != 0 {
		expires_in += fmt.Sprintf("%dh ", hours)
	}
	if minutes != 0 {
		expires_in += fmt.Sprintf("%dm ", minutes)
	}
	expires_in = strings.TrimSuffix(expires_in, " ")
	return
}

func (email *Emailer) init(app *appContext) {
	email.fromAddr = app.config.Section("email").Key("address").String()
	email.fromName = app.config.Section("email").Key("from").String()
	email.sendMethod = app.config.Section("email").Key("method").String()
	if email.sendMethod == "mailgun" {
		email.mg = mailgun.NewMailgun(strings.Split(email.fromAddr, "@")[1], app.config.Section("mailgun").Key("api_key").String())
		api_url := app.config.Section("mailgun").Key("api_url").String()
		// Mailgun client takes the base url, so we need to trim off the end (e.g 'v3/messages'
		if strings.Contains(api_url, "messages") {
			api_url = api_url[0:strings.LastIndex(api_url, "/")]
			api_url = api_url[0:strings.LastIndex(api_url, "/")]
		}
		email.mg.SetAPIBase(api_url)
	} else if email.sendMethod == "smtp" {
		app.host = app.config.Section("smtp").Key("server").String()
		email.smtpAuth = smtp.PlainAuth("", email.fromAddr, app.config.Section("smtp").Key("password").String(), app.host)
	}
}

func (email *Emailer) constructInvite(code string, invite Invite, app *appContext) error {
	email.content.subject = app.config.Section("invite_emails").Key("subject").String()
	expiry := invite.ValidTill
	d, t, expires_in := email.formatExpiry(expiry, false, app.datePattern, app.timePattern)
	message := app.config.Section("email").Key("message").String()
	invite_link := app.config.Section("invite_emails").Key("url_base").String()
	invite_link = fmt.Sprintf("%s/%s", invite_link, code)

	for _, key := range []string{"html", "text"} {
		fpath := app.config.Section("invite_emails").Key("email_" + key).String()
		tpl, err := template.ParseFiles(fpath)
		if err != nil {
			return err
		}
		var tplData bytes.Buffer
		err = tpl.Execute(&tplData, map[string]string{
			"expiry_date": d,
			"expiry_time": t,
			"expires_in":  expires_in,
			"invite_link": invite_link,
			"message":     message,
		})
		if err != nil {
			return err
		}
		if key == "html" {
			email.content.html = tplData.String()
		} else {
			email.content.text = tplData.String()
		}
	}
	email.sendType = "invite"
	return nil
}

func (email *Emailer) constructExpiry(code string, invite Invite, app *appContext) error {
	email.content.subject = "Notice: Invite expired"
	expiry := app.formatDatetime(invite.ValidTill)
	for _, key := range []string{"html", "text"} {
		fpath := app.config.Section("notifications").Key("expiry_" + key).String()
		tpl, err := template.ParseFiles(fpath)
		if err != nil {
			return err
		}
		var tplData bytes.Buffer
		err = tpl.Execute(&tplData, map[string]string{
			"code":   code,
			"expiry": expiry,
		})
		if err != nil {
			return err
		}
		if key == "html" {
			email.content.html = tplData.String()
		} else {
			email.content.text = tplData.String()
		}
	}
	email.sendType = "expiry"
	return nil
}

func (email *Emailer) constructCreated(code, username, address string, invite Invite, app *appContext) error {
	email.content.subject = "Notice: User created"
	created := app.formatDatetime(invite.Created)
	var tplAddress string
	if app.config.Section("email").Key("no_username").MustBool(false) {
		tplAddress = "n/a"
	} else {
		tplAddress = address
	}
	for _, key := range []string{"html", "text"} {
		fpath := app.config.Section("notifications").Key("created_" + key).String()
		tpl, err := template.ParseFiles(fpath)
		if err != nil {
			return err
		}
		var tplData bytes.Buffer
		err = tpl.Execute(&tplData, map[string]string{
			"code":     code,
			"username": username,
			"address":  tplAddress,
			"time":     created,
		})
		if err != nil {
			return err
		}
		if key == "html" {
			email.content.html = tplData.String()
		} else {
			email.content.text = tplData.String()
		}
	}
	email.sendType = "created"
	return nil
}

func (email *Emailer) constructReset(pwr Pwr, app *appContext) error {
	email.content.subject = app.config.Section("password_resets").Key("subject").MustString("Password reset - Jellyfin")
	d, t, expires_in := email.formatExpiry(pwr.Expiry, true, app.datePattern, app.timePattern)
	message := app.config.Section("email").Key("message").String()
	for _, key := range []string{"html", "text"} {
		fpath := app.config.Section("password_resets").Key("email_" + key).String()
		tpl, err := template.ParseFiles(fpath)
		if err != nil {
			return err
		}
		var tplData bytes.Buffer
		err = tpl.Execute(&tplData, map[string]string{
			"username":    pwr.Username,
			"expiry_date": d,
			"expiry_time": t,
			"expires_in":  expires_in,
			"pin":         pwr.Pin,
			"message":     message,
		})
		if err != nil {
			return err
		}
		if key == "html" {
			email.content.html = tplData.String()
		} else {
			email.content.text = tplData.String()
		}
	}
	email.sendType = "reset"
	return nil
}

func (email *Emailer) send(address string, app *appContext) error {
	if email.sendMethod == "mailgun" {
		message := email.mg.NewMessage(
			fmt.Sprintf("%s <%s>", email.fromName, email.fromAddr),
			email.content.subject,
			email.content.text,
			address)
		message.SetHtml(email.content.html)
		mgapp, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()
		_, _, err := email.mg.Send(mgapp, message)
		if err != nil {
			return err
		}
	} else if email.sendMethod == "smtp" {
		e := jEmail.NewEmail()
		e.Subject = email.content.subject
		e.From = fmt.Sprintf("%s <%s>", email.fromName, email.fromAddr)
		e.To = []string{address}
		e.Text = []byte(email.content.text)
		e.HTML = []byte(email.content.html)
		smtpType := app.config.Section("smtp").Key("encryption").String()
		tlsConfig := &tls.Config{
			InsecureSkipVerify: false,
			ServerName:         app.host,
		}
		var err error
		if smtpType == "ssl_tls" {
			port := app.config.Section("smtp").Key("port").MustInt(465)
			server := fmt.Sprintf("%s:%d", app.host, port)
			err = e.SendWithTLS(server, email.smtpAuth, tlsConfig)
		} else if smtpType == "starttls" {
			port := app.config.Section("smtp").Key("port").MustInt(587)
			server := fmt.Sprintf("%s:%d", app.host, port)
			e.SendWithStartTLS(server, email.smtpAuth, tlsConfig)
		}
		return err
	}
	return nil
}
