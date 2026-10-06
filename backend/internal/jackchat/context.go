package jackchat

import "unicode/utf8"

// contextRefill is how much of the budget a conversation keeps after it
// overflows. Moving the start point in one larger step keeps the beginning of
// the request unchanged for the following turns, so prompt caching keeps
// working until the next overflow.
const contextRefill = 0.7

// ContextFullError is stored on a reply when the upstream rejected the request
// as longer than the model's context; the page shows a localised hint.
const ContextFullError = "context_length_exceeded"

// estimateTextTokens roughly counts tokens: about four ASCII characters or
// one CJK character per token.
func estimateTextTokens(s string) int {
	ascii, other := 0, 0
	for _, r := range s {
		if r < utf8.RuneSelf {
			ascii++
		} else {
			other++
		}
	}
	return ascii/4 + other + 4
}

// estimateMessageTokens estimates what a message costs in the model input.
func estimateMessageTokens(m Message) int {
	n := estimateTextTokens(m.Content)
	for _, a := range m.Attachments {
		switch a.Kind {
		case KindText:
			n += estimateTextTokens(a.ExtractedText)
		case KindPDF:
			// Text plus page images; about one token per 100 bytes, at least 1000.
			n += max(1000, int(a.Size/100))
		default:
			n += 1500
		}
	}
	return n
}

// selectContext returns the messages to send, starting at the conversation's
// saved start point. When they exceed budget it moves the start forward to a
// user turn so that what remains fits within contextRefill of the budget. The
// last user turn is always kept. omitted counts the messages left out.
func selectContext(history []Message, start *int64, budget int) (kept []Message, newStart *int64, omitted int) {
	from := 0
	if start != nil {
		for i, m := range history {
			if m.ID >= *start {
				from = i
				break
			}
			from = i + 1
		}
		if from >= len(history) {
			from = 0
		}
	}
	total := 0
	for _, m := range history[from:] {
		total += estimateMessageTokens(m)
	}
	if budget > 0 && total > budget {
		target := int(float64(budget) * contextRefill)
		sum := 0
		cut := len(history) - 1
		for i := len(history) - 1; i >= from; i-- {
			sum += estimateMessageTokens(history[i])
			if sum > target && i < len(history)-1 {
				break
			}
			cut = i
		}
		// Start on a user turn so the model never sees a reply without its question.
		for cut < len(history)-1 && history[cut].Role != RoleUser {
			cut++
		}
		for cut > 0 && history[cut].Role != RoleUser {
			cut--
		}
		from = cut
		id := history[from].ID
		newStart = &id
	}
	return history[from:], newStart, from
}
