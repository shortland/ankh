package slack

import (
	"fmt"
	"os/user"
	"strings"

	ankh "github.com/appnexus/ankh/context"
	"github.com/appnexus/ankh/util"
	"github.com/nlopes/slack"
)

const DEFAULT_ICON_URL = "https://github.com/appnexus/ankh/blob/master/ankh.png?raw=true"
const DEFAULT_USERNAME = "ankh"

// Send out a release message based on the chart, version and environment
// supplied by the user
func PingSlackChannel(ctx *ankh.ExecutionContext, ankhFile *ankh.AnkhFile) error {

	// attempt the connection
	api := slack.New(ctx.AnkhConfig.Slack.Token)

	// get environment from env vs. context
	envOrContext := util.GetEnvironmentOrContext(ctx.Environment, ctx.Context)

	var messages []string
	for i := 0; i < len(ankhFile.Charts); i++ {
		chart := &ankhFile.Charts[i]
		message, err := getMessageText(ctx, chart, envOrContext)
		if err != nil {
			return fmt.Errorf("Unable to prompt for slack message. Using default value. Error: %v", err)
		} else {
			messages = append(messages, message)
		}
	}
	messageText := strings.Join(messages, "\n")

	pretext := ctx.AnkhConfig.Slack.Pretext
	if pretext == "" {
		pretext = "A new release notification has been received"
	}

	attachment := slack.Attachment{
		Color:   "good",
		Pretext: pretext,
		Text:    messageText,
	}

	icon := DEFAULT_ICON_URL
	if ctx.AnkhConfig.Slack.Icon != "" {
		icon = ctx.AnkhConfig.Slack.Icon
	}

	username := DEFAULT_USERNAME
	if ctx.AnkhConfig.Slack.Username != "" {
		username = ctx.AnkhConfig.Slack.Username
	}

	messageParams := slack.PostMessageParameters{
		IconURL:  icon,
		Username: username,
	}

	if !ctx.DryRun {
		channelId, err := getSlackChannelIDByName(api, ctx.SlackChannel)
		if err != nil {
			return err
		}

		_, _, err = api.PostMessage(channelId, slack.MsgOptionAttachments(attachment), slack.MsgOptionPostMessageParameters(messageParams))
		return err
	} else {
		ctx.Logger.Infof("--dry-run set so not sending message '%v' to slack channel %v", messageText, ctx.SlackChannel)
	}

	return nil
}

func getSlackChannelIDByName(api *slack.Client, channelName string) (string, error) {

	params := slack.GetConversationsParameters{}
	params.ExcludeArchived = "true"
	params.Limit = 1000

	// Look for public channels and private channels the bot was invited to
	params.Types = []string{"public_channel", "private_channel"}

	channels, nextCursor, err := api.GetConversations(&params)
	if err != nil || channels == nil {
		return "", err
	}

	// Look for channel
	for _, channel := range channels {
		if channel.Name == channelName {
			return channel.ID, nil
		}
	}

	// If it doesn't exist and there are more channels, keep going
	for nextCursor != "" {
		channels, nextCursor, err = api.GetConversations(&params)
		params.Cursor = nextCursor
		for _, channel := range channels {
			if channel.Name == channelName {
				return channel.ID, nil
			}
		}
	}

	return "", fmt.Errorf("channel %v not found", channelName)
}

func getMessageText(ctx *ankh.ExecutionContext, chart *ankh.Chart, envOrContext string) (string, error) {

	// Override takes precedence
	if ctx.SlackMessageOverride != "" {
		return ctx.SlackMessageOverride, nil
	}

	// If format is set, use that
	format := ctx.AnkhConfig.Slack.Format
	if ctx.Mode == ankh.Rollback {
		format = ctx.AnkhConfig.Slack.RollbackFormat
	}

	versionString := ""
	if chart.Tag != nil {
		versionString = *chart.Tag
	}

	chartString, err := util.GetChartString(chart)
	if err != nil {
		return "", err
	}

	if format != "" {
		message, err := util.ReplaceFormatVariables(format, chartString, versionString, envOrContext)
		if err != nil {
			ctx.Logger.Infof("Unable to use format: '%v'. Will prompt for message", format)
		} else {
			return message, nil
		}
	}

	// Otherwise, prompt for message
	message, err := promptForMessageText(chartString, versionString, envOrContext)
	if err != nil {
		ctx.Logger.Infof("Unable to prompt for message. Will use default message")
	}

	return message, nil
}

func promptForMessageText(chart string, version string, envOrContext string) (string, error) {
	currentUser, err := user.Current()
	if err != nil {
		return "", err
	}
	defaultMessage := fmt.Sprintf("%v is releasing %v@%v to *%v*", currentUser.Username, chart, version, envOrContext)
	if envOrContext == "rollback" {
		defaultMessage = fmt.Sprintf("%v is rolling back %v in *%v*", currentUser, chart, envOrContext)
	}

	message, err := util.PromptForInput(defaultMessage, "Slack Message")
	if err != nil {
		return defaultMessage, err
	}

	return message, nil
}
