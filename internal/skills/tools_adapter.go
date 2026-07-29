package skills

import (
	"context"
	"errors"

	coretools "github.com/mohamedmasoud3030-tech/mosaid/internal/tools"
)

type CoreToolAdapter struct {
	Registry *coretools.Registry
}

func (a CoreToolAdapter) InvokeTool(ctx context.Context, call ToolCall) (any, error) {
	if a.Registry == nil {
		return nil, errors.New("core tool registry unavailable")
	}
	result, err := a.Registry.Execute(ctx, coretools.Request{
		Name:          call.Name,
		Arguments:     call.Arguments,
		Mode:          call.Mode,
		UserID:        call.UserID,
		Resource:      call.Resource,
		ApprovalToken: call.ApprovalToken,
	})
	if err != nil {
		return nil, err
	}
	if result.Approval != nil {
		return result.Approval, ErrApprovalRequired
	}
	return result.Value, nil
}
