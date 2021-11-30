package main

import (
	"net/http"
	"net/url"

	"github.com/DisgoOrg/disgo/core"
	"github.com/DisgoOrg/disgo/discord"
	"github.com/DisgoOrg/disgo/rest"
	"github.com/DisgoOrg/disgo/webhook"
)

type WebhookCreateState struct {
	Interaction *core.SlashCommandInteraction
	Subreddit   string
}

func webhookCreateHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	code := query.Get("code")
	state := discord.Snowflake(query.Get("state"))
	guildID := query.Get("guild_id")
	if code == "" || state == "" || guildID == "" {
		writeMessage(w, http.StatusBadRequest, `missing info<br />Retry or reach out <a href="https://discord.gg/sD3ABd5" target="_blank">here</a> for help`)
		return
	}

	webhookState, ok := states[state]
	if !ok {
		writeMessage(w, http.StatusForbidden, `state not found or expired<br />Retry or reach out <a href="https://discord.gg/sD3ABd5" target="_blank">here</a> for help`)
		return
	}
	delete(states, state)

	compiledRoute, _ := tokenURL.Compile(nil)
	var rs *struct {
		*w.Webhook `json:"webhook"`
	}

	rq := url.Values{
		"client_id":     {},
		"client_secret": {secret},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {baseURL + CreateCallbackURL},
	}
	var err error
	err = disgo.RestServices.RestClient().Do(compiledRoute, rq, &rs)
	if err != nil {
		logger.Errorf("error while exchanging code: %s", err)
		writeError(w)
		return
	}

	webhookClient := webhook.NewClient(rs.Webhook.ID(), *rs.Webhook.Token,
		webhook.WithRestClientConfigOpts(
			rest.WithHTTPClient(httpClient),
		),
		webhook.WithLogger(logger),
	)
	if err != nil {
		logger.Errorf("error creating webhook client: %s", err)
		writeError(w)
		return
	}

	go func() {
		database.Create(&SubredditSubscription{
			Subreddit:    webhookState.Subreddit,
			GuildID:      *rs.Webhook.GuildID,
			ChannelID:    *rs.Webhook.ChannelID,
			WebhookID:    webhookClient.ID(),
			WebhookToken: webhookClient.Token(),
		})
	}()

	subscribeToSubreddit(webhookState.Subreddit, webhookClient)

	_, err = webhookClient.SendMessage(wapi.NewWebhookMessageCreateBuilder().
		SetContent("Webhook for [r/" + webhookState.Subreddit + "](https://www.reddit.com/r/" + webhookState.Subreddit + ") successfully created").
		Build(),
	)
	message := api.NewMessageCreateBuilder().SetEphemeral(true)
	if err != nil {
		logger.Errorf("error while tesing webhook: %s", err)
		message.SetContent("There was a problem setting up your webhook.\nRetry or reach out for help [here](https://discord.gg/sD3ABd5)")
	} else {
		message.SetContent("Successfully added webhook. Everything is ready to go")
	}

	_, err = webhookState.Interaction.SendFollowup(message.Build())
	if err != nil {
		logger.Errorf("error while sending followup: %s", err)
		writeError(w)
		return
	}

	http.Redirect(w, r, baseURL+SuccessURL, http.StatusSeeOther)
}

func webhookCreateSuccessHandler(w http.ResponseWriter, _ *http.Request) {
	writeMessage(w, http.StatusOK, `subreddit successfully created.<br />You can now close this site<br /><br />For further questions you can reach out <a href="https://discord.gg/sD3ABd5" target="_blank">here</a>`)
}

func writeError(w http.ResponseWriter) {
	writeMessage(w, http.StatusInternalServerError, `There was a problem setting up your subreddit notifications<br />Retry or reach out <a href="https://discord.gg/sD3ABd5" target="_blank">here</a> for help`)
}

func writeMessage(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(message))
}
