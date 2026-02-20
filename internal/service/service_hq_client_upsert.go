package service

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"strings"

	"sophia/internal/model"
)

func (c *hqClient) UpsertCR(ctx context.Context, repoID, uid string, request model.HQUpsertCRRequest) (*model.HQUpsertCRResponse, error) {
	endpoint, err := c.urlFor(path.Join("api", "v1", "repos", url.PathEscape(strings.TrimSpace(repoID)), "crs", url.PathEscape(strings.TrimSpace(uid))))
	if err != nil {
		return nil, err
	}
	var response model.HQUpsertCRResponse
	if err := c.doJSON(ctx, http.MethodPut, endpoint, request, &response); err != nil {
		return nil, err
	}
	if strings.TrimSpace(response.CRUID) == "" {
		response.CRUID = strings.TrimSpace(uid)
	}
	return &response, nil
}
