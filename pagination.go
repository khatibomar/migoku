package main

import (
	"net/http"
	"strconv"
)

const (
	defaultPageSize = 50
	maxPageSize     = 500
)

type PaginationParams struct {
	Page     int
	PageSize int
	Offset   int
}

type PaginatedResponse struct {
	Data       any            `json:"data"`
	Pagination PaginationMeta `json:"pagination"`
}

type PaginationMeta struct {
	Page       int  `json:"page"`
	PageSize   int  `json:"page_size"`
	Total      int  `json:"total,omitempty"`
	TotalPages int  `json:"total_pages,omitempty"`
	HasNext    bool `json:"has_next"`
	HasPrev    bool `json:"has_prev"`
}

func parsePaginationParams(r *http.Request) PaginationParams {
	page := 1
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	pageSize := defaultPageSize
	if sizeStr := r.URL.Query().Get("page_size"); sizeStr != "" {
		if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 {
			pageSize = min(s, maxPageSize)
		}
	}

	offset := (page - 1) * pageSize

	return PaginationParams{
		Page:     page,
		PageSize: pageSize,
		Offset:   offset,
	}
}

func buildPaginationMeta(params PaginationParams, total int) PaginationMeta {
	totalPages := 0
	if total > 0 {
		totalPages = (total + params.PageSize - 1) / params.PageSize
	}

	return PaginationMeta{
		Page:       params.Page,
		PageSize:   params.PageSize,
		Total:      total,
		TotalPages: totalPages,
		HasNext:    params.Page < totalPages,
		HasPrev:    params.Page > 1,
	}
}

func (app *Application) respondPaginated(w http.ResponseWriter, r *http.Request, data any, params PaginationParams, total int) {
	response := PaginatedResponse{
		Data:       data,
		Pagination: buildPaginationMeta(params, total),
	}
	app.respondJSON(w, r, response)
}
