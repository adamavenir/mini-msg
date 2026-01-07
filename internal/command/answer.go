package command

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	answerHeaderStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("111"))
	answerQuestionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true)
	answerOptionStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("157"))
	answerProStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("78"))
	answerConStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	answerMetaStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	answerPromptStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	answerSkipStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
)

// qaPair tracks a question and its answer during the session.
type qaPair struct {
	question types.Question
	answer   string
}

// NewAnswerCmd creates the answer command.
func NewAnswerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "answer [question-id] [answer-text]",
		Short: "Answer questions",
		Long: `Answer questions interactively or directly.

Interactive mode (for humans):
  fray answer              Review and answer all open questions one at a time

Direct mode (for agents):
  fray answer <qstn-id> "answer text" --as agent
                           Answer a specific question directly

In interactive mode:
  - Type a letter (a, b, c) to select a proposed option
  - Type your own answer
  - Press 's' to skip the question for now
  - Press 'q' to quit

Skipped questions are offered for review at the end of the session.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentRef, _ := cmd.Flags().GetString("as")

			// Direct mode: answer <qstn-id> "answer" --as agent
			if len(args) >= 2 {
				if agentRef == "" {
					return writeCommandError(cmd, fmt.Errorf("--as is required for direct answer mode"))
				}
				return runDirectAnswer(ctx, args[0], args[1], agentRef)
			}

			// Interactive mode: answer (uses username from config)
			if len(args) == 0 {
				return runInteractiveAnswer(ctx, agentRef)
			}

			// Single arg could be question ID with missing answer
			return writeCommandError(cmd, fmt.Errorf("usage: fray answer <question-id> \"answer\" --as agent\n       fray answer (interactive mode)"))
		},
	}

	cmd.Flags().StringP("as", "", "", "agent identity (required for direct mode)")
	return cmd
}

// runDirectAnswer handles: fray answer <qstn-id> "answer" --as agent
func runDirectAnswer(ctx *CommandContext, questionRef, answerText, agentRef string) error {
	agentID, err := resolveAgentRef(ctx, agentRef)
	if err != nil {
		return err
	}

	agent, err := db.GetAgent(ctx.DB, agentID)
	if err != nil {
		return err
	}
	if agent == nil {
		return fmt.Errorf("agent not found: @%s", agentID)
	}
	if agent.LeftAt != nil {
		return fmt.Errorf("agent @%s has left. Use 'fray back @%s' to resume", agentID, agentID)
	}

	question, err := resolveQuestionRef(ctx.DB, questionRef)
	if err != nil {
		return err
	}

	if question.Status == types.QuestionStatusClosed {
		return fmt.Errorf("question %s is already closed", question.GUID)
	}
	if question.Status == types.QuestionStatusAnswered {
		return fmt.Errorf("question %s is already answered", question.GUID)
	}

	// For direct mode, post single Q&A formatted message
	pairs := []qaPair{{question: *question, answer: answerText}}
	if err := postAnswerSummary(ctx.DB, ctx.Project.DBPath, agentID, pairs); err != nil {
		return err
	}

	if ctx.JSONMode {
		payload := map[string]any{
			"question_id": question.GUID,
			"answered_by": agentID,
			"answer":      answerText,
		}
		return json.NewEncoder(os.Stdout).Encode(payload)
	}

	fmt.Printf("Answered %s\n", question.GUID)
	return nil
}

// runInteractiveAnswer handles: fray answer (interactive mode for humans)
func runInteractiveAnswer(ctx *CommandContext, agentRef string) error {
	// Determine identity: --as flag or username from config
	var identity string
	if agentRef != "" {
		resolved, err := resolveAgentRef(ctx, agentRef)
		if err != nil {
			return err
		}
		identity = resolved
	} else {
		// Use username from config (human user)
		username, err := db.GetConfig(ctx.DB, "username")
		if err != nil {
			return err
		}
		if username == "" {
			return fmt.Errorf("no username configured. Use 'fray chat' first or specify --as")
		}
		identity = username
	}

	// Get open questions addressed to this identity
	targetedQuestions, err := db.GetQuestions(ctx.DB, &types.QuestionQueryOptions{
		Statuses: []types.QuestionStatus{types.QuestionStatusOpen},
		ToAgent:  &identity,
	})
	if err != nil {
		return err
	}

	// Also get open questions with no target (anyone can answer)
	untargetedQuestions, err := db.GetQuestions(ctx.DB, &types.QuestionQueryOptions{
		Statuses:     []types.QuestionStatus{types.QuestionStatusOpen},
		NoTargetOnly: true,
	})
	if err != nil {
		return err
	}

	// Combine: targeted first, then untargeted
	questions := append(targetedQuestions, untargetedQuestions...)

	if len(questions) == 0 {
		fmt.Printf("No open questions for @%s\n", identity)
		return nil
	}

	return runAnswerSessionTUI(ctx.DB, ctx.Project.DBPath, identity, questions)
}

// questionSet groups questions by their source message.
type questionSet struct {
	askedIn   *string
	createdAt int64
	questions []types.Question
}

func groupQuestionSets(questions []types.Question) []questionSet {
	// Group by asked_in
	groups := make(map[string]*questionSet)
	var nilGroup *questionSet

	for _, q := range questions {
		key := ""
		if q.AskedIn != nil {
			key = *q.AskedIn
		}

		if key == "" {
			if nilGroup == nil {
				nilGroup = &questionSet{createdAt: q.CreatedAt}
			}
			nilGroup.questions = append(nilGroup.questions, q)
		} else {
			if groups[key] == nil {
				groups[key] = &questionSet{askedIn: q.AskedIn, createdAt: q.CreatedAt}
			}
			groups[key].questions = append(groups[key].questions, q)
		}
	}

	// Convert to slice and sort by creation time (newest first)
	var sets []questionSet
	for _, g := range groups {
		sets = append(sets, *g)
	}
	if nilGroup != nil {
		sets = append(sets, *nilGroup)
	}

	sort.Slice(sets, func(i, j int) bool {
		return sets[i].createdAt > sets[j].createdAt
	})

	return sets
}

// postAnswerSummary posts a single message with all Q&A pairs formatted nicely.
func postAnswerSummary(database *sql.DB, dbPath string, identity string, pairs []qaPair) error {
	now := time.Now().Unix()

	// Collect unique askers
	askers := make(map[string]struct{})
	for _, pair := range pairs {
		askers[pair.question.FromAgent] = struct{}{}
	}
	askerList := make([]string, 0, len(askers))
	for asker := range askers {
		askerList = append(askerList, "@"+asker)
	}

	// Build the summary message body in parseable format
	var body strings.Builder
	body.WriteString(fmt.Sprintf("answered %s\n\n", strings.Join(askerList, " ")))

	for _, pair := range pairs {
		// Question
		body.WriteString(fmt.Sprintf("Q: %s\n", pair.question.Re))

		// Answer with indented multi-line support
		answerLines := strings.Split(pair.answer, "\n")
		body.WriteString(fmt.Sprintf("A: %s\n", answerLines[0]))
		for _, line := range answerLines[1:] {
			body.WriteString(fmt.Sprintf("   %s\n", line))
		}
		body.WriteString("\n")
	}

	bodyStr := strings.TrimSpace(body.String())

	// Extract mentions from all answers
	bases, _ := db.GetAgentBases(database)
	mentions := core.ExtractMentions(bodyStr, bases)
	mentions = core.ExpandAllMention(mentions, bases)

	// Determine home (use first question's thread if any)
	home := ""
	if len(pairs) > 0 && pairs[0].question.ThreadGUID != nil {
		home = *pairs[0].question.ThreadGUID
	}

	// Create the summary message
	created, err := db.CreateMessage(database, types.Message{
		TS:        now,
		FromAgent: identity,
		Body:      bodyStr,
		Mentions:  mentions,
		Home:      home,
	})
	if err != nil {
		return err
	}

	if err := db.AppendMessage(dbPath, created); err != nil {
		return err
	}

	// Update agent last seen (if this is an agent, not a user)
	agent, _ := db.GetAgent(database, identity)
	if agent != nil {
		updates := db.AgentUpdates{LastSeen: types.OptionalInt64{Set: true, Value: &now}}
		_ = db.UpdateAgent(database, identity, updates)
	}

	// Update all questions to point to this summary message
	statusValue := string(types.QuestionStatusAnswered)
	for _, pair := range pairs {
		updated, err := db.UpdateQuestion(database, pair.question.GUID, db.QuestionUpdates{
			Status:     types.OptionalString{Set: true, Value: &statusValue},
			AnsweredIn: types.OptionalString{Set: true, Value: &created.ID},
		})
		if err != nil {
			return err
		}

		if err := db.AppendQuestionUpdate(dbPath, db.QuestionUpdateJSONLRecord{
			GUID:       updated.GUID,
			Status:     &statusValue,
			AnsweredIn: &created.ID,
		}); err != nil {
			return err
		}
	}

	return nil
}

func printSummary(answered, stillSkipped int) {
	if answered == 0 && stillSkipped == 0 {
		fmt.Println("\nNo questions answered.")
		return
	}

	summary := fmt.Sprintf("\nDone! Answered %d question(s)", answered)
	if stillSkipped > 0 {
		summary += fmt.Sprintf(", %d still skipped", stillSkipped)
	}
	summary += "."
	fmt.Println(summary)
}
