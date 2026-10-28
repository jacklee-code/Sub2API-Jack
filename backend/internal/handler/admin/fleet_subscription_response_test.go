//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/jackfleet"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type fleetListRepo struct{ scopedSubscriptionRepo }

func (r *fleetListRepo) ListByUserID(ctx context.Context, _ int64) ([]service.UserSubscription, error) {
	sub, err := r.GetByID(ctx, 1)
	return []service.UserSubscription{*sub}, err
}
func (r *fleetListRepo) ListByGroupID(ctx context.Context, _ int64, _ pagination.PaginationParams) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	subs, err := r.ListByUserID(ctx, 2)
	return subs, &pagination.PaginationResult{Total: 1, Page: 1, PageSize: 20, Pages: 1}, err
}
func (r *fleetListRepo) List(ctx context.Context, p pagination.PaginationParams, _, _ *int64, _, _, _, _ string) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	return r.ListByGroupID(ctx, 3, p)
}

// Exercise the three conflicted call sites, including the actual metadata query
// and upstream projection. Neither side of the merge may replace the other.
func TestFleetSubscriptionListsRespectScopedResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"all", "group", "user"} {
		for _, mode := range []string{"admin", "read", "write", "query_error"} {
			t.Run(endpoint+"/"+mode, func(t *testing.T) {
				db, mock, err := sqlmock.New()
				require.NoError(t, err)
				defer db.Close()
				expectation := mock.ExpectQuery("SELECT m.subscription_id,m.fleet_id,m.independent_expiry").WithArgs(sqlmock.AnyArg())
				if mode == "query_error" {
					expectation.WillReturnError(errors.New("fixture database error"))
				} else {
					expectation.WillReturnRows(sqlmock.NewRows([]string{"subscription_id", "fleet_id", "independent_expiry"}).AddRow(1, 7, true))
				}
				svc := service.NewSubscriptionService(nil, &fleetListRepo{}, nil, nil, nil)
				defer svc.Stop()
				h := NewSubscriptionHandler(svc)
				h.fleet = &FleetHandler{store: jackfleet.NewStore(db)}
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest("GET", "/subscriptions", nil)
				c.Params = gin.Params{{Key: "id", Value: "2"}}
				if mode == "read" || mode == "write" {
					permissions := []string{"subscriptions.extend"}
					if mode == "read" {
						permissions = append(permissions, "subscriptions.read")
					}
					c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{Policy: adminaccess.Policy{GlobalPermissions: permissions}}))
				}
				switch endpoint {
				case "all":
					h.List(c)
				case "group":
					h.ListByGroup(c)
				case "user":
					h.ListByUser(c)
				}
				require.NoError(t, mock.ExpectationsWereMet())
				if mode == "query_error" {
					require.Equal(t, 500, w.Code)
					return
				}
				require.Equal(t, 200, w.Code, w.Body.String())
				var response struct {
					Data json.RawMessage `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				list := response.Data
				if endpoint != "user" {
					var page struct {
						Items json.RawMessage `json:"items"`
					}
					require.NoError(t, json.Unmarshal(response.Data, &page))
					list = page.Items
				}
				var items []map[string]any
				require.NoError(t, json.Unmarshal(list, &items))
				require.Len(t, items, 1)
				item := items[0]
				require.Equal(t, float64(1), item["id"])
				if mode == "admin" {
					require.Equal(t, float64(7), item["fleet_id"])
					require.Equal(t, true, item["fleet_independent_expiry"])
					require.Contains(t, item, "user")
				} else {
					for _, key := range []string{"user", "group", "assigned_by_user", "fleet_id", "fleet_independent_expiry"} {
						require.NotContains(t, item, key)
					}
					if mode == "write" {
						require.Len(t, item, 1)
					} else {
						require.Contains(t, item, "daily_usage_usd")
					}
				}
			})
		}
	}
}
