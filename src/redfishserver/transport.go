package redfishserver

import (
	"context"
	"encoding/json"
	"github.com/go-kit/kit/log"
	httptransport "github.com/go-kit/kit/transport/http"
	"github.com/gorilla/mux"
	"net/http"
)

const HTTP_HEADER_SERVER = "go-redfish/0.1"

// errorer is implemented by all concrete response types that may contain
// errors. It allows us to change the HTTP response code without needing to
// trigger an endpoint (transport-level) error. For more information, read the
// big comment in endpoints.go.
type errorer interface {
	error() error
}

func NewRedfishHandler(svc Service, logger log.Logger) http.Handler {
	r := mux.NewRouter()
	options := []httptransport.ServerOption{
		httptransport.ServerAfter(httptransport.SetContentType("application/json;charset=utf-8")),
		httptransport.ServerAfter(httptransport.SetResponseHeader("OData-Version", "4.0")),
		httptransport.ServerAfter(httptransport.SetResponseHeader("Server", HTTP_HEADER_SERVER)),
		httptransport.ServerErrorLogger(logger),
		httptransport.ServerErrorEncoder(encodeError),
	}

	r.Handle("/redfish", http.RedirectHandler("/redfish/", 308))
	r.Handle("/redfish/v1", http.RedirectHandler("/redfish/v1/", 308))

	r.Methods("GET").Path("/redfish/").Handler(
		httptransport.NewServer(
			makeRawJSONRedfishGetEndpoint(svc),
			decodeRedfishGetRequest,
			encodeResponse,
			options...,
		))

	r.Methods("GET").Path("/redfish/v1/").Handler(
		httptransport.NewServer(
			makeRawJSONRedfishGetEndpoint(svc),
			decodeRedfishGetRequest,
			encodeResponse,
			options...,
		))

	r.Methods("GET").Path("/redfish/v1/Systems").Handler(
		httptransport.NewServer(
			makeRawJSONRedfishGetEndpoint(svc),
			decodeRedfishGetRequest,
			encodeResponse,
			options...,
		))

	r.Methods("GET").Path("/redfish/v1/Systems/{system}").Handler(
		httptransport.NewServer(
			makeRawJSONRedfishGetEndpoint(svc),
			decodeRedfishGetRequest,
			encodeResponse,
			options...,
		))

	return r
}

func checkHeaders(header http.Header) (err error) {
	// TODO: check Content-Type (for things with request body only)
	// TODO: check OData-MaxVersion "Indicates the maximum version of OData
	//                              that an odata-aware client understands"
	// TODO: check OData-Version "Services shall reject requests which specify
	//                              an unsupported OData version. If a service
	//                              encounters a version that it does not
	//                              support, the service should reject the
	//                              request with status code [412]
	//                              (#status-412). If client does not specify
	//                              an Odata-Version header, the client is
	//                              outside the boundaries of this
	//                              specification."

	return
}

// we are basically tied to HTTP, so just pass the request down to the function
// don't anticipate ever adding grpc or other support here, so this should be fine for now
// if we do add, we'll have to simply re-work the function parameters.
func decodeRedfishGetRequest(_ context.Context, r *http.Request) (dec interface{}, err error) {
	// need to decode headers that we may need manually

	err = checkHeaders(r.Header)
	if err != nil {
		return nil, err
	}

	headers := make(map[string]string)
	if ver := r.Header.Get("OData-Version"); ver != "" {
		headers["OData-Version"] = ver
	}

	route := mux.CurrentRoute(r)
	pathTemplate, rerr := route.GetPathTemplate()
	if rerr != nil {
		pathTemplate, rerr = route.GetPathRegexp()
	}
	dec = templatedRedfishGetRequest{url: r.URL.Path, args: mux.Vars(r), pathTemplate: pathTemplate}
	return dec, nil
}

// probably could do something cool with channels and goroutines here so that
// we dont buffer the entire response, but not worth the effort at this moment
func encodeResponse(ctx context.Context, w http.ResponseWriter, response interface{}) error {

	// if needed:
	//w.Header().Set("x-header-name", "header value")

	// TODO: Cache-Control
	// TODO: Max-Forwards (SHOULD)
	// TODO: Access-Control-Allow-Origin (SHALL)
	// TODO: Allow - (SHALL) - returns GET/PUT/POST/PATCH/DELETE/HEAD

	// TODO: ETAG
	// TODO: Link
	// TODO: CORS headers
	//w.Header().Set("Access-Control-Allow-Origin", "*")
	//w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	//w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type")

	if e, ok := response.(errorer); ok && e.error() != nil {
		encodeError(ctx, e.error(), w)
		return nil
	}

	decoded := response.(templatedRedfishGetResponse)

	switch output := decoded.output.(type) {
	case []byte:
		_, err := w.Write(output)
		return err
	default:
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}
	// for when we switch to structured output
	// return json.NewEncoder(w).Encode(output)

	return nil
}

func encodeError(_ context.Context, err error, w http.ResponseWriter) {
	if err == nil {
		panic("encodeError with nil error")
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(codeFrom(err))
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": err.Error(),
	})
}

func codeFrom(err error) int {
	switch err {
	case ErrNotFound:
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}