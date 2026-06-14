package handlers

import (
	"github.com/labstack/echo/v4"

	"github.com/kurodakayn/mpp-backend/internal/dto"
)

func sendError(c echo.Context, code int, errCode, message string) error {
	resp := dto.ErrorResponse{}
	resp.Error.Code = errCode
	resp.Error.Message = message
	return c.JSON(code, resp)
}
