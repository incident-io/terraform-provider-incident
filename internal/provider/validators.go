package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

type NonEmptyListValidator struct{}

func (n NonEmptyListValidator) ValidateList(ctx context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if req.ConfigValue.IsNull() {
		return
	} else if len(req.ConfigValue.Elements()) == 0 {
		resp.Diagnostics.AddError("List cannot be empty", fmt.Sprintf("%s cannot be empty", req.Path.String()))
		return
	}
}

func (n NonEmptyListValidator) Description(ctx context.Context) string {
	return "List cannot be empty"
}

func (n NonEmptyListValidator) MarkdownDescription(ctx context.Context) string {
	return "List cannot be empty"
}
