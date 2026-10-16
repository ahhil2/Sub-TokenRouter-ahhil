package identity

import (
	"context"
	"slices"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

type MemberGroup struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	MemberCount int64  `json:"member_count"`
}

type GroupMember struct {
	UserID         int64    `json:"user_id"`
	Username       string   `json:"username"`
	Email          string   `json:"email"`
	IdentityGroups []string `json:"identity_groups"`
}

type MemberGroupRepository interface {
	ListGroups(context.Context) ([]MemberGroup, error)
	ListMembers(context.Context, string, string, int, int) ([]GroupMember, int64, error)
	ChangeMembers(context.Context, int64, string, []int64, bool) error
	Memberships(context.Context, []int64) (map[int64][]string, error)
}

type MemberGroups struct {
	Repository MemberGroupRepository
	Invalidate func(context.Context, int64)
}

func MemberGroupCategory(code string) (string, error) {
	switch code {
	case "bonus_5", "bonus_10":
		return "recharge", nil
	case "ahhilai":
		return "pricing", nil
	default:
		return "", apperror.BadRequest("INVALID_IDENTITY_GROUP", "invalid identity group")
	}
}

func (service *MemberGroups) Change(ctx context.Context, actorID int64, code string, userIDs []int64, add bool) error {
	if _, err := MemberGroupCategory(code); err != nil {
		return err
	}
	if len(userIDs) == 0 || len(userIDs) > 500 {
		return apperror.BadRequest("INVALID_IDENTITY_MEMBERS", "select between 1 and 500 users")
	}
	userIDs = slices.Clone(userIDs)
	slices.Sort(userIDs)
	userIDs = slices.Compact(userIDs)
	if userIDs[0] <= 0 {
		return apperror.BadRequest("INVALID_IDENTITY_MEMBERS", "invalid user ID")
	}
	if err := service.Repository.ChangeMembers(ctx, actorID, code, userIDs, add); err != nil {
		return err
	}
	if service.Invalidate != nil {
		for _, userID := range userIDs {
			service.Invalidate(ctx, userID)
		}
	}
	return nil
}

func (service *MemberGroups) Members(ctx context.Context, code, search string, page, size int) ([]GroupMember, int64, error) {
	if code != "" {
		if _, err := MemberGroupCategory(code); err != nil {
			return nil, 0, err
		}
	}
	return service.Repository.ListMembers(ctx, code, strings.TrimSpace(search), max(page, 1), min(max(size, 1), 100))
}
