// Package llm joins passthrough paths onto an upstream base. It does not rewrite a body that is already in the provider protocol.
package llm

import (
	"net/url"
	"path"
	"strings"
)

// PassthroughURL is the outbound address produced by LiteLLM _join_url_paths.
// It first joins the subpath onto the api_base path and rejects "..".
// If an OpenAI result still has no /v1/, v1 is inserted after api.openai.com/.
func PassthroughURL(base, endpoint, provider string) string {
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	if endpoint != "" && !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}
	u.Path = joinBaseAndEndpoint(u.Path, endpoint)
	u.RawPath = ""
	out := u.String()
	if (provider == "openai" || provider == "openai_passthrough") && !strings.Contains(out, "/v1/") {
		out = strings.Replace(out, "api.openai.com/", "api.openai.com/v1/", 1)
	}
	return out
}

// PassthroughSubpath matches HttpPassThroughEndpointHelpers.construct_target_url_with_subpath.
// When include is false or the subpath is empty, base is returned unchanged. Otherwise the subpath is normalized and joined.
func PassthroughSubpath(base, subpath string, include bool) string {
	if !include || subpath == "" {
		return base
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	subpath = strings.TrimPrefix(subpath, "/")
	trailing := strings.HasSuffix(subpath, "/")
	safe := strings.TrimPrefix(path.Clean("/"+subpath), "/")
	if safe == "." {
		safe = ""
	}
	if trailing && safe != "" && !strings.HasSuffix(safe, "/") {
		safe += "/"
	}
	return base + safe
}

// joinBaseAndEndpoint joins an upstream base and an endpoint path without a double slash and without dropping a prefix already on the base.
func joinBaseAndEndpoint(basePath, endpointPath string) string {
	trailing := strings.HasSuffix(endpointPath, "/")
	if basePath == "" || basePath == "/" {
		normalized := path.Clean("/" + strings.TrimPrefix(endpointPath, "/"))
		if trailing && normalized != "/" {
			normalized += "/"
		}
		return normalized
	}
	basePath = strings.TrimRight(basePath, "/")
	combined := path.Clean(basePath + "/" + strings.TrimPrefix(endpointPath, "/"))
	if combined != basePath && !strings.HasPrefix(combined, basePath+"/") {
		return basePath + "/"
	}
	if trailing && !strings.HasSuffix(combined, "/") {
		combined += "/"
	}
	return combined
}
