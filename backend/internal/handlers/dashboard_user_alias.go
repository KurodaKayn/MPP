package handlers

import (
	userdashboard "github.com/kurodakayn/mpp-backend/internal/handlers/user-dashboard"
	dashboardsvc "github.com/kurodakayn/mpp-backend/internal/services/dashboard"
)

type UserDashboardHandler = userdashboard.Handler

func NewUserDashboardHandler(s *dashboardsvc.DashboardService) *UserDashboardHandler {
	return userdashboard.New(s)
}
