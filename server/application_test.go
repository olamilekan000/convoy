package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/frain-dev/convoy/config"
	"github.com/frain-dev/convoy/server/models"
	pager "github.com/gobeam/mongo-go-pagination"

	"github.com/frain-dev/convoy"
	"github.com/frain-dev/convoy/mocks"
	"github.com/go-chi/chi/v5"
	"github.com/golang/mock/gomock"
	"github.com/sebdah/goldie/v2"
)

func verifyMatch(t *testing.T, w httptest.ResponseRecorder) {
	g := goldie.New(
		t,
		goldie.WithDiffEngine(goldie.ColoredDiff),
	)
	g.Assert(t, t.Name(), w.Body.Bytes())
}

func stripTimestamp(t *testing.T, obj string, b *bytes.Buffer) *bytes.Buffer {
	var res serverResponse
	err := json.NewDecoder(b).Decode(&res)
	if err != nil {
		t.Errorf("could not stripTimestamp: %s", err)
		t.FailNow()
	}

	if res.Data == nil {
		return bytes.NewBuffer([]byte(``))
	}

	switch obj {
	case "application":
		var a convoy.Application
		err := json.Unmarshal(res.Data, &a)
		if err != nil {
			t.Errorf("could not stripTimestamp: %s", err)
			t.FailNow()
		}

		a.UID = ""
		a.CreatedAt, a.UpdatedAt, a.DeletedAt = 0, 0, 0

		jsonData, err := json.Marshal(a)
		if err != nil {
			t.Error(err)
		}

		return bytes.NewBuffer(jsonData)
	case "endpoint":
		var e convoy.Endpoint
		err := json.Unmarshal(res.Data, &e)
		if err != nil {
			t.Errorf("could not stripTimestamp: %s", err)
			t.FailNow()
		}

		e.UID = ""
		e.CreatedAt, e.UpdatedAt, e.DeletedAt = 0, 0, 0

		jsonData, err := json.Marshal(e)
		if err != nil {
			t.Error(err)
		}

		return bytes.NewBuffer(jsonData)
	default:
		t.Errorf("invalid data body - %v of type %T", obj, obj)
		t.FailNow()
	}

	return nil
}

func TestApplicationHandler_GetApp(t *testing.T) {

	var app *applicationHandler

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	group := mocks.NewMockGroupRepository(ctrl)
	apprepo := mocks.NewMockApplicationRepository(ctrl)
	msgRepo := mocks.NewMockMessageRepository(ctrl)

	groupID := "1234567890"

	validID := "123456789"

	app = newApplicationHandler(msgRepo, apprepo, group)

	tt := []struct {
		name       string
		cfgPath    string
		method     string
		statusCode int
		id         string
		dbFn       func(appRepo *mocks.MockApplicationRepository, orgRepo *mocks.MockGroupRepository)
	}{
		{
			name:       "app not found",
			cfgPath:    "./testdata/Auth_Config/basic-convoy.json",
			method:     http.MethodGet,
			statusCode: http.StatusNotFound,
			id:         "12345",
			dbFn: func(appRepo *mocks.MockApplicationRepository, orgRepo *mocks.MockGroupRepository) {
				appRepo.EXPECT().
					FindApplicationByID(gomock.Any(), gomock.Any()).
					Return(nil, convoy.ErrApplicationNotFound).Times(1)
			},
		},
		{
			name:       "valid application",
			cfgPath:    "./testdata/Auth_Config/basic-convoy.json",
			method:     http.MethodGet,
			statusCode: http.StatusOK,
			id:         validID,
			dbFn: func(appRepo *mocks.MockApplicationRepository, orgRepo *mocks.MockGroupRepository) {
				appRepo.EXPECT().
					FindApplicationByID(gomock.Any(), gomock.Any()).Times(1).
					Return(&convoy.Application{
						UID:       validID,
						GroupID:   groupID,
						Title:     "Valid application",
						Endpoints: []convoy.Endpoint{},
					}, nil)

			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			url := fmt.Sprintf("/v1/applications/%s", tc.id)
			req := httptest.NewRequest(tc.method, url, nil)
			req.SetBasicAuth("test", "test")
			w := httptest.NewRecorder()
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", tc.id)

			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			// Arrange Expectations
			if tc.dbFn != nil {
				tc.dbFn(apprepo, group)
			}

			err := config.LoadConfig(tc.cfgPath)
			if err != nil {
				t.Error("Failed to load config file")
			}

			router := buildRoutes(app)

			// Act
			router.ServeHTTP(w, req)

			// Assert
			if w.Code != tc.statusCode {
				t.Errorf("Want status '%d', got '%d'", tc.statusCode, w.Code)
			}

			verifyMatch(t, *w)
		})
	}

}

func TestApplicationHandler_GetApps(t *testing.T) {

	var app *applicationHandler

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	group := mocks.NewMockGroupRepository(ctrl)
	apprepo := mocks.NewMockApplicationRepository(ctrl)
	msgRepo := mocks.NewMockMessageRepository(ctrl)

	groupID := "1234567890"

	validID := "123456789"

	app = newApplicationHandler(msgRepo, apprepo, group)

	tt := []struct {
		name       string
		cfgPath    string
		method     string
		statusCode int
		dbFn       func(appRepo *mocks.MockApplicationRepository, orgRepo *mocks.MockGroupRepository)
	}{
		{
			name:       "valid applications",
			cfgPath:    "./testdata/Auth_Config/basic-convoy.json",
			method:     http.MethodGet,
			statusCode: http.StatusOK,
			dbFn: func(appRepo *mocks.MockApplicationRepository, orgRepo *mocks.MockGroupRepository) {
				appRepo.EXPECT().
					LoadApplicationsPaged(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).
					Return([]convoy.Application{
						{
							UID:       validID,
							GroupID:   groupID,
							Title:     "Valid application - 0",
							Endpoints: []convoy.Endpoint{},
						},
					}, pager.PaginationData{}, nil)

			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			req := httptest.NewRequest(tc.method, "/v1/applications", nil)
			req.SetBasicAuth("test", "test")
			w := httptest.NewRecorder()

			pageable := models.Pageable{
				Page:    1,
				PerPage: 10,
			}
			req = req.WithContext(context.WithValue(req.Context(), pageableCtx, pageable))

			// Arrange Expectations.
			if tc.dbFn != nil {
				tc.dbFn(apprepo, group)
			}

			err := config.LoadConfig(tc.cfgPath)
			if err != nil {
				t.Error("Failed to load config file")
			}

			router := buildRoutes(app)

			// Act
			router.ServeHTTP(w, req)

			if w.Code != tc.statusCode {
				t.Errorf("Want status '%d', got '%d'", tc.statusCode, w.Code)
			}

			verifyMatch(t, *w)
		})
	}

}

func TestApplicationHandler_CreateApp(t *testing.T) {

	groupID := "1234567890"
	group := &convoy.Group{
		UID: groupID,
	}

	bodyReader := strings.NewReader(`{ "group_id": "` + groupID + `", "name": "ABC_DEF_TEST", "secret": "12345" }`)

	tt := []struct {
		name       string
		cfgPath    string
		method     string
		statusCode int
		body       *strings.Reader
		dbFn       func(app *applicationHandler)
	}{
		{
			name:       "invalid request",
			cfgPath:    "./testdata/Auth_Config/basic-convoy.json",
			method:     http.MethodPost,
			statusCode: http.StatusBadRequest,
			body:       strings.NewReader(``),
			dbFn: func(app *applicationHandler) {
				o, _ := app.groupRepo.(*mocks.MockGroupRepository)
				o.EXPECT().
					LoadGroups(gomock.Any()).Times(1).
					Return([]*convoy.Group{group}, nil)
			},
		},
		{
			name:       "invalid request - no app name",
			cfgPath:    "./testdata/Auth_Config/basic-convoy.json",
			method:     http.MethodPost,
			statusCode: http.StatusBadRequest,
			body:       strings.NewReader(`{}`),
			dbFn: func(app *applicationHandler) {
				o, _ := app.groupRepo.(*mocks.MockGroupRepository)
				o.EXPECT().
					LoadGroups(gomock.Any()).Times(1).
					Return([]*convoy.Group{group}, nil)
			},
		},
		{
			name:       "valid application",
			cfgPath:    "./testdata/Auth_Config/basic-convoy.json",
			method:     http.MethodPost,
			statusCode: http.StatusCreated,
			body:       bodyReader,
			dbFn: func(app *applicationHandler) {
				a, _ := app.appRepo.(*mocks.MockApplicationRepository)
				a.EXPECT().
					CreateApplication(gomock.Any(), gomock.Any()).Times(1).
					Return(nil)

				o, _ := app.groupRepo.(*mocks.MockGroupRepository)
				o.EXPECT().
					LoadGroups(gomock.Any()).Times(1).
					Return([]*convoy.Group{group}, nil)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			var app *applicationHandler

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			group := mocks.NewMockGroupRepository(ctrl)
			apprepo := mocks.NewMockApplicationRepository(ctrl)
			msgRepo := mocks.NewMockMessageRepository(ctrl)

			app = newApplicationHandler(msgRepo, apprepo, group)

			// Arrange
			req := httptest.NewRequest(tc.method, "/v1/applications", tc.body)
			req.SetBasicAuth("test", "test")
			req.Header.Add("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// Arrange Expectations
			if tc.dbFn != nil {
				tc.dbFn(app)
			}

			err := config.LoadConfig(tc.cfgPath)
			if err != nil {
				t.Error("Failed to load config file")
			}

			router := buildRoutes(app)

			// Act.
			router.ServeHTTP(w, req)

			if w.Code != tc.statusCode {
				t.Errorf("Want status '%d', got '%d'", tc.statusCode, w.Code)
			}

			d := stripTimestamp(t, "application", w.Body)

			w.Body = d
			verifyMatch(t, *w)
		})
	}

}

func TestApplicationHandler_UpdateApp(t *testing.T) {

	groupID := "1234567890"

	appId := "12345"
	bodyReader := strings.NewReader(`{"name": "ABC_DEF_TEST_UPDATE"}`)

	tt := []struct {
		name       string
		cfgPath    string
		method     string
		statusCode int
		appId      string
		body       *strings.Reader
		dbFn       func(app *applicationHandler)
	}{
		{
			name:       "invalid request",
			cfgPath:    "./testdata/Auth_Config/basic-convoy.json",
			method:     http.MethodPut,
			statusCode: http.StatusBadRequest,
			appId:      appId,
			body:       strings.NewReader(``),
			dbFn: func(app *applicationHandler) {
				a, _ := app.appRepo.(*mocks.MockApplicationRepository)
				a.EXPECT().
					FindApplicationByID(gomock.Any(), gomock.Any()).Times(1).
					Return(&convoy.Application{
						UID:       appId,
						GroupID:   groupID,
						Title:     "Valid application update",
						Endpoints: []convoy.Endpoint{},
					}, nil)
			},
		},
		{
			name:       "invalid request - no app name",
			cfgPath:    "./testdata/Auth_Config/basic-convoy.json",
			method:     http.MethodPut,
			statusCode: http.StatusBadRequest,
			appId:      appId,
			body:       strings.NewReader(`{}`),
			dbFn: func(app *applicationHandler) {
				a, _ := app.appRepo.(*mocks.MockApplicationRepository)
				a.EXPECT().
					FindApplicationByID(gomock.Any(), gomock.Any()).Times(1).
					Return(&convoy.Application{
						UID:       appId,
						GroupID:   groupID,
						Title:     "Valid application update",
						Endpoints: []convoy.Endpoint{},
					}, nil)
			},
		},
		{
			name:       "valid request - update secret",
			cfgPath:    "./testdata/Auth_Config/basic-convoy.json",
			method:     http.MethodPut,
			statusCode: http.StatusAccepted,
			appId:      appId,
			body:       strings.NewReader(`{ "name": "ABC", "secret": "xyz" }`),
			dbFn: func(app *applicationHandler) {
				a, _ := app.appRepo.(*mocks.MockApplicationRepository)
				a.EXPECT().
					FindApplicationByID(gomock.Any(), gomock.Any()).Times(1).
					Return(&convoy.Application{
						UID:       appId,
						GroupID:   groupID,
						Title:     "Valid application update",
						Endpoints: []convoy.Endpoint{},
					}, nil)

				a.EXPECT().
					UpdateApplication(gomock.Any(), gomock.Any()).Times(1).
					Return(nil)
			},
		},
		{
			name:       "valid request - update support email",
			cfgPath:    "./testdata/Auth_Config/basic-convoy.json",
			method:     http.MethodPut,
			statusCode: http.StatusAccepted,
			appId:      appId,
			body:       strings.NewReader(`{ "name": "ABC", "support_email": "engineering@frain.dev" }`),
			dbFn: func(app *applicationHandler) {
				a, _ := app.appRepo.(*mocks.MockApplicationRepository)
				a.EXPECT().
					UpdateApplication(gomock.Any(), gomock.Any()).Times(1).
					Return(nil)

				a.EXPECT().
					FindApplicationByID(gomock.Any(), gomock.Any()).Times(1).
					Return(&convoy.Application{
						UID:       appId,
						GroupID:   groupID,
						Title:     "Valid application update",
						Endpoints: []convoy.Endpoint{},
					}, nil)
			},
		},
		{
			name:       "valid application update",
			cfgPath:    "./testdata/Auth_Config/basic-convoy.json",
			method:     http.MethodPut,
			statusCode: http.StatusAccepted,
			appId:      appId,
			body:       bodyReader,
			dbFn: func(app *applicationHandler) {
				a, _ := app.appRepo.(*mocks.MockApplicationRepository)
				a.EXPECT().
					UpdateApplication(gomock.Any(), gomock.Any()).Times(1).
					Return(nil)

				a.EXPECT().
					FindApplicationByID(gomock.Any(), gomock.Any()).Times(1).
					Return(&convoy.Application{
						UID:       appId,
						GroupID:   groupID,
						Title:     "Valid application update",
						Endpoints: []convoy.Endpoint{},
					}, nil)

			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var app *applicationHandler

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			group := mocks.NewMockGroupRepository(ctrl)
			apprepo := mocks.NewMockApplicationRepository(ctrl)
			msgRepo := mocks.NewMockMessageRepository(ctrl)

			app = newApplicationHandler(msgRepo, apprepo, group)

			url := fmt.Sprintf("/v1/applications/%s", tc.appId)
			req := httptest.NewRequest(tc.method, url, tc.body)
			req.SetBasicAuth("test", "test")
			req.Header.Add("Content-Type", "application/json")

			w := httptest.NewRecorder()
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("appID", tc.appId)

			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
			req = req.WithContext(context.WithValue(req.Context(), appCtx, &convoy.Application{
				UID:       appId,
				GroupID:   groupID,
				Title:     "Valid application update",
				Endpoints: []convoy.Endpoint{},
			}))

			if tc.dbFn != nil {
				tc.dbFn(app)
			}

			err := config.LoadConfig(tc.cfgPath)
			if err != nil {
				t.Error("Failed to load config file")
			}

			router := buildRoutes(app)

			// Act.
			router.ServeHTTP(w, req)

			if w.Code != tc.statusCode {
				t.Errorf("Want status '%d', got '%d'", tc.statusCode, w.Code)
			}

			verifyMatch(t, *w)
		})
	}

}

func TestApplicationHandler_CreateAppEndpoint(t *testing.T) {

	var app *applicationHandler

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	group := mocks.NewMockGroupRepository(ctrl)
	apprepo := mocks.NewMockApplicationRepository(ctrl)
	msgRepo := mocks.NewMockMessageRepository(ctrl)

	groupID := "1234567890"

	bodyReader := strings.NewReader(`{"url": "https://google.com", "description": "Test"}`)

	app = newApplicationHandler(msgRepo, apprepo, group)

	appId := "123456789"

	tt := []struct {
		name       string
		cfgPath    string
		method     string
		statusCode int
		appId      string
		body       *strings.Reader
		dbFn       func(appRepo *mocks.MockApplicationRepository, orgRepo *mocks.MockGroupRepository)
	}{
		{
			name:       "valid endpoint",
			cfgPath:    "./testdata/Auth_Config/basic-convoy.json",
			method:     http.MethodPost,
			statusCode: http.StatusCreated,
			appId:      appId,
			body:       bodyReader,
			dbFn: func(appRepo *mocks.MockApplicationRepository, orgRepo *mocks.MockGroupRepository) {
				appRepo.EXPECT().
					UpdateApplication(gomock.Any(), gomock.Any()).Times(1).
					Return(nil)

				appRepo.EXPECT().
					FindApplicationByID(gomock.Any(), gomock.Any()).Times(2).
					Return(&convoy.Application{
						UID:       appId,
						GroupID:   groupID,
						Title:     "Valid application endpoint",
						Endpoints: []convoy.Endpoint{},
					}, nil)

			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			url := fmt.Sprintf("/v1/applications/%s/endpoints", tc.appId)
			req := httptest.NewRequest(tc.method, url, tc.body)
			req.SetBasicAuth("test", "test")
			req.Header.Add("Content-Type", "application/json")
			w := httptest.NewRecorder()

			if tc.dbFn != nil {
				tc.dbFn(apprepo, group)
			}

			err := config.LoadConfig(tc.cfgPath)
			if err != nil {
				t.Error("Failed to load config file")
			}

			router := buildRoutes(app)

			// Act
			router.ServeHTTP(w, req)

			if w.Code != tc.statusCode {
				t.Errorf("Want status '%d', got '%d'", tc.statusCode, w.Code)
			}
		})
	}

}

func TestApplicationHandler_UpdateAppEndpoint(t *testing.T) {

	groupID := "1234567890"

	appId := "12345"
	endpointId := "9999900000-8888"

	tt := []struct {
		name       string
		cfgPath    string
		method     string
		statusCode int
		appId      string
		endpointId string
		body       *strings.Reader
		dbFn       func(app *applicationHandler)
	}{
		{
			name:       "invalid request",
			cfgPath:    "./testdata/Auth_Config/basic-convoy.json",
			method:     http.MethodPut,
			statusCode: http.StatusBadRequest,
			appId:      appId,
			endpointId: endpointId,
			body:       strings.NewReader(``),
			dbFn: func(app *applicationHandler) {
				a, _ := app.appRepo.(*mocks.MockApplicationRepository)
				a.EXPECT().
					UpdateApplication(gomock.Any(), gomock.Any()).Times(0).
					Return(nil)

				a.EXPECT().
					FindApplicationByID(gomock.Any(), gomock.Any()).Times(1).
					Return(&convoy.Application{
						UID:       appId,
						GroupID:   groupID,
						Title:     "invalid application update",
						Endpoints: []convoy.Endpoint{},
					}, nil)

			},
		},
		{
			name:       "valid application",
			cfgPath:    "./testdata/Auth_Config/basic-convoy.json",
			method:     http.MethodPut,
			statusCode: http.StatusAccepted,
			appId:      appId,
			endpointId: endpointId,
			body:       strings.NewReader(`{"url": "https://google.com", "description": "Correct endpoint"}`),
			dbFn: func(app *applicationHandler) {
				a, _ := app.appRepo.(*mocks.MockApplicationRepository)
				a.EXPECT().
					UpdateApplication(gomock.Any(), gomock.Any()).Times(1).
					Return(nil)

				a.EXPECT().
					FindApplicationByID(gomock.Any(), gomock.Any()).Times(1).
					Return(&convoy.Application{
						UID:     appId,
						GroupID: groupID,
						Title:   "Valid application update",
						Endpoints: []convoy.Endpoint{
							{
								UID:         endpointId,
								TargetURL:   "http://",
								Description: "desc",
							},
						},
					}, nil)

			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var app *applicationHandler

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			group := mocks.NewMockGroupRepository(ctrl)
			apprepo := mocks.NewMockApplicationRepository(ctrl)
			msgRepo := mocks.NewMockMessageRepository(ctrl)

			app = newApplicationHandler(msgRepo, apprepo, group)

			url := fmt.Sprintf("/v1/applications/%s/endpoints/%s", tc.appId, tc.endpointId)
			req := httptest.NewRequest(tc.method, url, tc.body)
			req.SetBasicAuth("test", "test")
			req.Header.Add("Content-Type", "application/json")

			w := httptest.NewRecorder()
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("appID", tc.appId)
			rctx.URLParams.Add("endpointID", tc.endpointId)

			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
			req = req.WithContext(context.WithValue(req.Context(), appCtx, &convoy.Application{
				UID:       appId,
				GroupID:   groupID,
				Title:     "Valid application update",
				Endpoints: []convoy.Endpoint{},
			}))

			// Arrange Expectations
			if tc.dbFn != nil {
				tc.dbFn(app)
			}

			err := config.LoadConfig(tc.cfgPath)
			if err != nil {
				t.Error("Failed to load config file")
			}

			router := buildRoutes(app)

			// Act
			router.ServeHTTP(w, req)

			if w.Code != tc.statusCode {
				t.Errorf("Want status '%d', got '%d'", tc.statusCode, w.Code)
			}

			d := stripTimestamp(t, "endpoint", w.Body)

			w.Body = d
			verifyMatch(t, *w)
		})
	}

}

func Test_applicationHandler_GetDashboardSummary(t *testing.T) {

	var app *applicationHandler

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	groupRepo := mocks.NewMockGroupRepository(ctrl)
	apprepo := mocks.NewMockApplicationRepository(ctrl)
	msgRepo := mocks.NewMockMessageRepository(ctrl)

	groupID := "1234567890"

	group := &convoy.Group{
		UID:  groupID,
		Name: "Valid group",
	}

	app = newApplicationHandler(msgRepo, apprepo, groupRepo)

	tt := []struct {
		name       string
		method     string
		statusCode int
		dbFn       func(msgRepo *mocks.MockMessageRepository, appRepo *mocks.MockApplicationRepository, orgRepo *mocks.MockGroupRepository)
	}{
		{
			name:       "valid groups",
			method:     http.MethodGet,
			statusCode: http.StatusOK,
			dbFn: func(msgRepo *mocks.MockMessageRepository, appRepo *mocks.MockApplicationRepository, orgRepo *mocks.MockGroupRepository) {
				appRepo.EXPECT().
					SearchApplicationsByGroupId(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).
					Return([]convoy.Application{
						{
							UID:       "validID",
							GroupID:   groupID,
							Title:     "Valid application - 0",
							Endpoints: []convoy.Endpoint{},
						},
					}, nil)
				msgRepo.EXPECT().
					LoadMessageIntervals(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).
					Return([]models.MessageInterval{
						{
							Data: models.MessageIntervalData{
								Interval: 12,
								Time:     "2020-10",
							},
							Count: 10,
						},
					}, nil)

			},
		},
	}

	format := "2006-01-02T15:04:05"

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, fmt.Sprintf("/v1/dashboard/%s/summary?startDate=%s&type=daily", groupID, time.Now().Format(format)), nil)
			responseRecorder := httptest.NewRecorder()
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("groupID", groupID)

			request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, rctx))
			request = request.WithContext(context.WithValue(request.Context(), groupCtx, group))

			if tc.dbFn != nil {
				tc.dbFn(msgRepo, apprepo, groupRepo)
			}

			fetchDashboardSummary(apprepo, msgRepo)(http.HandlerFunc(app.GetDashboardSummary)).
				ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != tc.statusCode {
				t.Errorf("Want status '%d', got '%d'", tc.statusCode, responseRecorder.Code)
			}
		})
	}

}

func Test_applicationHandler_GetPaginatedApps(t *testing.T) {

	var app *applicationHandler

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	org := mocks.NewMockGroupRepository(ctrl)
	apprepo := mocks.NewMockApplicationRepository(ctrl)
	msgRepo := mocks.NewMockMessageRepository(ctrl)

	groupID := "1234567890"

	group := &convoy.Group{
		UID:  groupID,
		Name: "Valid group",
	}

	app = newApplicationHandler(msgRepo, apprepo, org)

	tt := []struct {
		name       string
		method     string
		statusCode int
		dbFn       func(appRepo *mocks.MockApplicationRepository, orgRepo *mocks.MockGroupRepository)
	}{
		{
			name: "valid groups" +
				"",
			method:     http.MethodGet,
			statusCode: http.StatusOK,
			dbFn: func(appRepo *mocks.MockApplicationRepository, orgRepo *mocks.MockGroupRepository) {
				appRepo.EXPECT().
					LoadApplicationsPagedByGroupId(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).
					Return([]convoy.Application{
						{
							UID:       "validID",
							GroupID:   groupID,
							Title:     "Valid application - 0",
							Endpoints: []convoy.Endpoint{},
						},
					},
						pager.PaginationData{},
						nil)

			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, fmt.Sprintf("/v1/dashboard/%s/apps?page=1", groupID), nil)
			responseRecorder := httptest.NewRecorder()
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("groupID", groupID)

			request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, rctx))
			request = request.WithContext(context.WithValue(request.Context(), groupCtx, group))

			pageable := models.Pageable{
				Page:    1,
				PerPage: 10,
			}
			request = request.WithContext(context.WithValue(request.Context(), pageableCtx, pageable))

			if tc.dbFn != nil {
				tc.dbFn(apprepo, org)
			}

			fetchGroupApps(apprepo)(http.HandlerFunc(app.GetPaginatedApps)).
				ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != tc.statusCode {
				t.Errorf("Want status '%d', got '%d'", tc.statusCode, responseRecorder.Code)
			}

			verifyMatch(t, *responseRecorder)
		})
	}

}