package multiagent

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type hitlInterceptorKey struct{}

type HITLToolInterceptor func(ctx context.Context, toolName, arguments string) (string, error)

type humanRejectError struct {
	reason string
}

func (e *humanRejectError) Error() string {
	if strings.TrimSpace(e.reason) == "" {
		return "rejected by user"
	}
	return "rejected by user: " + strings.TrimSpace(e.reason)
}

func NewHumanRejectError(reason string) error {
	return &humanRejectError{reason: strings.TrimSpace(reason)}
}

func IsHumanRejectError(err error) bool {
	var target *humanRejectError
	return errors.As(err, &target)
}

func WithHITLToolInterceptor(ctx context.Context, fn HITLToolInterceptor) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, hitlInterceptorKey{}, fn)
}

// hitlToolCallMiddleware registers both Invokable and Streamable.
// Eino filesystem's execute is a streaming tool (StreamableTool); if only Invokable is registered,
// HITL will not intercept it and it will run directly.
func hitlToolCallMiddleware() compose.ToolMiddleware {
	return compose.ToolMiddleware{
		Invokable:  hitlInvokableToolCallMiddleware(),
		Streamable: hitlStreamableToolCallMiddleware(),
	}
}

func hitlClearReturnDirectlyIfTransfer(ctx context.Context, toolName string) {
	if !strings.EqualFold(strings.TrimSpace(toolName), adk.TransferToAgentToolName) {
		return
	}
	_ = clearADKReturnDirectly(ctx)
}

func hitlEditedArgumentsNotice(original, edited string) string {
	original = strings.TrimSpace(original)
	edited = strings.TrimSpace(edited)
	if edited == "" || edited == original {
		return ""
	}
	return "[HITL] Human reviewer approved this tool call with edited arguments.\n" +
		"Original arguments: " + original + "\n" +
		"Executed arguments: " + edited + "\n\n"
}

func hitlPrependEditedArgumentsNotice(result, original, edited string) string {
	notice := hitlEditedArgumentsNotice(original, edited)
	if notice == "" {
		return result
	}
	return notice + result
}

func hitlCollectStringStream(sr *schema.StreamReader[string]) (string, error) {
	if sr == nil {
		return "", nil
	}
	defer sr.Close()
	var b strings.Builder
	for {
		chunk, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			return b.String(), nil
		}
		if err != nil {
			return b.String(), err
		}
		b.WriteString(chunk)
	}
}

func hitlInvokableToolCallMiddleware() compose.InvokableToolMiddleware {
	return func(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
		return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
			originalArgs := ""
			editedArgs := ""
			if input != nil {
				if fn, ok := ctx.Value(hitlInterceptorKey{}).(HITLToolInterceptor); ok && fn != nil {
					originalArgs = input.Arguments
					edited, err := fn(ctx, input.Name, input.Arguments)
					if err != nil {
						if IsHumanRejectError(err) {
							// Human rejection should be a soft tool result so the model can continue iterating.
							// tool_search must remain JSON, otherwise the Eino toolsearch middleware will hard-crash ChatModel when parsing history.
							msg := HitlRejectToolResult(input.Name, err.Error())
							// transfer_to_agent is marked returnDirectly in Eino: after a successful tool call the ReAct
							// sub-graph goes directly to END, relying on SendToolGenAction inside the real tool to trigger
							// the handoff. When HITL rejects, the real tool is not executed; if the returnDirectly branch
							// is still taken, the supervisor would exit with no Transfer action and the model would not iterate.
							hitlClearReturnDirectlyIfTransfer(ctx, input.Name)
							return &compose.ToolOutput{Result: msg}, nil
						}
						return nil, err
					}
					if edited != "" {
						editedArgs = edited
						input.Arguments = edited
					}
				}
			}
			out, err := next(ctx, input)
			if err != nil || out == nil {
				return out, err
			}
			out.Result = hitlPrependEditedArgumentsNotice(out.Result, originalArgs, editedArgs)
			return out, nil
		}
	}
}

func hitlStreamableToolCallMiddleware() compose.StreamableToolMiddleware {
	return func(next compose.StreamableToolEndpoint) compose.StreamableToolEndpoint {
		return func(ctx context.Context, input *compose.ToolInput) (*compose.StreamToolOutput, error) {
			originalArgs := ""
			editedArgs := ""
			if input != nil {
				if fn, ok := ctx.Value(hitlInterceptorKey{}).(HITLToolInterceptor); ok && fn != nil {
					originalArgs = input.Arguments
					edited, err := fn(ctx, input.Name, input.Arguments)
					if err != nil {
						if IsHumanRejectError(err) {
							msg := HitlRejectToolResult(input.Name, err.Error())
							hitlClearReturnDirectlyIfTransfer(ctx, input.Name)
							return &compose.StreamToolOutput{
								Result: schema.StreamReaderFromArray([]string{msg}),
							}, nil
						}
						return nil, err
					}
					if edited != "" {
						editedArgs = edited
						input.Arguments = edited
					}
				}
			}
			out, err := next(ctx, input)
			if err != nil || out == nil {
				return out, err
			}
			if hitlEditedArgumentsNotice(originalArgs, editedArgs) == "" {
				return out, nil
			}
			result, collectErr := hitlCollectStringStream(out.Result)
			if collectErr != nil {
				return nil, collectErr
			}
			out.Result = schema.StreamReaderFromArray([]string{
				hitlPrependEditedArgumentsNotice(result, originalArgs, editedArgs),
			})
			return out, nil
		}
	}
}
