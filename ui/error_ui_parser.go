package ui

import (
	"context"
	"strconv"
	"strings"
	"suppa-ahg-stack/common-golang/validator"

	"github.com/invopop/ctxi18n/i18n"
)

type ErrorDataForUi struct {
	Item        string
	Description string
}

func FieldErrorsToUiErrors(ctx context.Context, fieldErrors []validator.ValidationError) []ErrorDataForUi {
	var result []ErrorDataForUi
	seen := make(map[string]bool)

	for _, fe := range fieldErrors {
		for key, msg := range fe.InvalidConditions {
			var desc string

			// Check if message uses the __ format (translation_key__param)
			if strings.Contains(msg, "__") {
				parts := strings.SplitN(msg, "__", 2)
				translationKey := "validation." + parts[0]

				if len(parts) > 1 && parts[1] != "" {
					// Try to parse param as int for %d formatting
					if paramInt, err := strconv.Atoi(parts[1]); err == nil {
						desc = i18n.T(ctx, translationKey, paramInt)
					} else {
						desc = i18n.T(ctx, translationKey, parts[1])
					}
				} else {
					desc = i18n.T(ctx, translationKey)
				}
			} else {
				// Map validation rule keys to translation keys
				switch key {
				case "required":
					desc = i18n.T(ctx, "validation.required")
				case "email":
					desc = i18n.T(ctx, "validation.email_invalid")
				case "min":
					desc = i18n.T(ctx, "validation.min")
				case "max":
					desc = i18n.T(ctx, "validation.max")
				case "in":
					desc = i18n.T(ctx, "validation.in")
				case "regex":
					desc = i18n.T(ctx, "validation.regex")
				default:
					desc = msg
				}
			}

			// Deduplicate by field+description
			dedupKey := fe.Field + ":" + desc
			if !seen[dedupKey] {
				seen[dedupKey] = true
				result = append(result, ErrorDataForUi{
					Item:        fe.Field,
					Description: desc,
				})
			}
		}
	}
	return result
}
