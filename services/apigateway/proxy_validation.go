package apigateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

// runRequestValidator enforces request validation rules when a requestValidatorId is
// configured on the method. When ValidateRequestParameters is set it checks that every
// required header/query/path parameter is present; when ValidateRequestBody is set it
// checks the body is valid JSON and, if the method declares a request model, validates
// it against the model's JSON Schema. Returns true if validation failed and the
// AWS-accurate 400 response has been written.
func (h *Handler) runRequestValidator(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	apiID string,
	method *Method,
	pathParams map[string]string,
) bool {
	rv, err := h.Backend.GetRequestValidator(apiID, method.RequestValidatorID)
	if err != nil {
		logger.Load(ctx).WarnContext(ctx, "APIGateway proxy: request validator not found",
			"validatorId", method.RequestValidatorID)

		return false // fail open when validator config is missing
	}

	if rv.ValidateRequestParameters {
		if missing := missingRequiredParameters(r, method, pathParams); len(missing) > 0 {
			writeValidationError(w, "Missing required request parameters: ["+strings.Join(missing, ", ")+"]")

			return true
		}
	}

	if rv.ValidateRequestBody {
		if denied := h.validateRequestBody(ctx, w, r, apiID, method); denied {
			return true
		}
	}

	return false
}

// validateRequestBody reads the request body (restoring it for downstream handlers),
// requires it to be valid JSON, and validates it against the method's request model
// schema when one is declared. Returns true when the AWS 400 body-validation response
// has been written.
func (h *Handler) validateRequestBody(
	ctx context.Context, w http.ResponseWriter, r *http.Request, apiID string, method *Method,
) bool {
	if r.Body == nil {
		return false
	}

	bodyBytes, readErr := io.ReadAll(http.MaxBytesReader(w, r.Body, maxProxyRequestBodyBytes))
	if readErr != nil {
		writeValidationError(w, "Invalid request body")

		return true
	}

	// Replace body so downstream handlers can still read it.
	r.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))

	if len(bodyBytes) == 0 || !json.Valid(bodyBytes) {
		writeValidationError(w, "Invalid request body")

		return true
	}

	schema := h.requestModelSchema(apiID, method, r.Header.Get("Content-Type"))
	if schema == "" {
		return false
	}

	if err := validateJSONAgainstSchema(bodyBytes, schema); err != nil {
		logger.Load(ctx).InfoContext(ctx, "APIGateway proxy: request body schema validation failed",
			"apiId", apiID, "error", err)
		writeValidationError(w, "Invalid request body")

		return true
	}

	return false
}

// requestModelSchema resolves the JSON Schema for the method's request model that
// matches the request content type (falling back to application/json), or "" when the
// method declares no usable model.
func (h *Handler) requestModelSchema(apiID string, method *Method, contentType string) string {
	if len(method.RequestModels) == 0 {
		return ""
	}

	ct := contentType
	if idx := strings.IndexByte(ct, ';'); idx >= 0 {
		ct = ct[:idx]
	}
	ct = strings.TrimSpace(ct)

	modelName := method.RequestModels[ct]
	if modelName == "" {
		modelName = method.RequestModels[contentTypeJSON]
	}
	if modelName == "" || strings.EqualFold(modelName, "Empty") {
		return ""
	}

	model, err := h.Backend.GetModel(apiID, modelName)
	if err != nil || model == nil {
		return ""
	}

	return model.Schema
}

// writeValidationError writes the AWS API Gateway request-validation 400 response:
// a JSON body of {"message": "..."} with the BAD_REQUEST error type header.
func writeValidationError(w http.ResponseWriter, message string) {
	w.Header().Set(headerContentType, "application/json")
	w.Header().Set("X-Amzn-Errortype", "BadRequestException")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{jsonMessageKey: message})
}

// missingRequiredParameters returns the names of required request parameters (as
// declared on the method's RequestParameters map) that are absent from the request.
// Keys are "method.request.{location}.{name}" and map to whether the parameter is
// required. Returned names are sorted for a stable error message.
func missingRequiredParameters(r *http.Request, method *Method, pathParams map[string]string) []string {
	var missing []string
	query := r.URL.Query()

	for spec, required := range method.RequestParameters {
		if !required {
			continue
		}

		location, name, ok := parseParameterSpec(spec)
		if !ok {
			continue
		}

		if !requestHasParameter(r, query, pathParams, location, name) {
			missing = append(missing, spec)
		}
	}

	sort.Strings(missing)

	return missing
}

// parseParameterSpec splits a "method.request.{location}.{name}" spec into its
// location ("header"/"querystring"/"path") and parameter name. The final bool reports
// whether the spec was well-formed.
func parseParameterSpec(spec string) (string, string, bool) {
	const prefix = "method.request."
	rest, found := strings.CutPrefix(spec, prefix)
	if !found {
		return "", "", false
	}
	loc, name, found := strings.Cut(rest, ".")
	if !found || name == "" {
		return "", "", false
	}

	return loc, name, true
}

// requestHasParameter reports whether the request carries a non-empty value for the
// given parameter location and name.
func requestHasParameter(
	r *http.Request, query url.Values, pathParams map[string]string, location, name string,
) bool {
	switch location {
	case paramLocationHeader:
		return r.Header.Get(name) != ""
	case paramLocationQuery:
		return query.Has(name)
	case paramLocationPath:
		_, ok := pathParams[name]

		return ok
	default:
		return true
	}
}
