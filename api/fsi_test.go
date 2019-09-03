package api

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/qri-io/dataset"
	"github.com/qri-io/qri/fsi"
	"github.com/qri-io/qri/lib"
	"github.com/qri-io/qri/repo"
)

func TestFSIHandlers(t *testing.T) {
	node, teardown := newTestNode(t)
	defer teardown()

	inst := newTestInstanceWithProfileFromNode(node)
	h := NewFSIHandlers(inst, false)

	// TODO (b5) - b/c of the way our API snapshotting we have to write these
	// folders to relative paths :( bad!
	checkoutDir := "fsi_tests/family_relationships"
	initDir := "fsi_tests/init_dir"
	if err := os.MkdirAll(initDir, os.ModePerm); err != nil {
		panic(err)
	}
	defer os.RemoveAll(filepath.Join("fsi_tests"))

	initCases := []handlerTestCase{
		{"OPTIONS", "/", nil},
		{"GET", "/", nil},
		{"POST", "/", nil},
		{"POST", fmt.Sprintf("/?filepath=%s", initDir), nil},
		{"POST", fmt.Sprintf("/?filepath=%s&name=api_test_init_dataset", initDir), nil},
		{"POST", fmt.Sprintf("/?filepath=%s&name=api_test_init_dataset&format=csv", initDir), nil},
		{"DELETE", "/", nil},
	}
	runHandlerTestCases(t, "init", h.InitHandler(""), initCases, true)

	statusCases := []handlerTestCase{
		{"OPTIONS", "/", nil},
		// TODO (b5) - can't ask for an FSI-linked status b/c the responses change with
		// temp directory names
		{"GET", "/me/movies", nil},
		{"DELETE", "/", nil},
	}
	runHandlerTestCases(t, "status", h.StatusHandler(""), statusCases, true)

	checkoutCases := []handlerTestCase{
		{"OPTIONS", "/", nil},
		{"POST", "/me/movies", nil},
		// TODO (b5) - can't ask for an FSI-linked status b/c the responses change with
		// temp directory names
		{"POST", fmt.Sprintf("/me/movies?dir=%s", checkoutDir), nil},
		{"DELETE", "/", nil},
	}
	runHandlerTestCases(t, "checkout", h.CheckoutHandler(""), checkoutCases, true)
}

func TestNoHistory(t *testing.T) {
	node, teardown := newTestNode(t)
	defer teardown()

	inst := newTestInstanceWithProfileFromNode(node)

	tmpDir := os.TempDir()
	initSubdir := "fsi_init_dir"
	initDir := filepath.Join(tmpDir, initSubdir)
	if err := os.MkdirAll(initDir, os.ModePerm); err != nil {
		panic(err)
	}
	defer os.RemoveAll(initDir)

	// Create a linked dataset without saving, it has no versions in the repository
	f := fsi.NewFSI(node.Repo)
	ref, err := f.InitDataset(fsi.InitParams{
		Filepath: initDir,
		Name:     "test_ds",
		Format:   "csv",
	})
	if err != nil {
		t.Fatal(err)
	}

	if ref != "peer/test_ds" {
		t.Errorf("expected ref to be \"peer/test_ds\", got \"%s\"", ref)
	}

	// Get mtimes for the component files
	st, _ := os.Stat(filepath.Join(initDir, "meta.json"))
	metaMtime := st.ModTime().Format(time.RFC3339)
	st, _ = os.Stat(filepath.Join(initDir, "schema.json"))
	schemaMtime := st.ModTime().Format(time.RFC3339)
	st, _ = os.Stat(filepath.Join(initDir, "body.csv"))
	bodyMtime := st.ModTime().Format(time.RFC3339)

	dsHandler := NewDatasetHandlers(inst, false)

	// Dataset with no history
	actualStatusCode, actualBody := APICall("/peer/test_ds", dsHandler.GetHandler)
	if actualStatusCode != 422 {
		t.Errorf("expected status code 422, got %d", actualStatusCode)
	}
	expectBody := `{ "meta": { "code": 422, "error": "no history" }, "data": null }`
	if expectBody != actualBody {
		t.Errorf("expected body %s, got %s", expectBody, actualBody)
	}

	// Dataset with no history, but FSI working directory has contents
	actualStatusCode, actualBody = APICall("/peer/test_ds?fsi=true", dsHandler.GetHandler)
	if actualStatusCode != 200 {
		t.Errorf("expected status code 200, got %d", actualStatusCode)
	}
	// Handle tempoary directory by replacing the temp part with a shorter string.
	resultBody := strings.Replace(actualBody, initDir, initSubdir, -1)
	expectBody = `{"data":{"peername":"peer","name":"test_ds","fsiPath":"fsi_init_dir","dataset":{"bodyPath":"fsi_init_dir/body.csv","meta":{"keywords":[],"qri":"md:0"},"name":"test_ds","peername":"peer","qri":"ds:0","structure":{"format":"csv","qri":"st:0","schema":{"items":{"items":[{"title":"name","type":"string"},{"title":"describe","type":"string"},{"title":"quantity","type":"integer"}],"type":"array"},"type":"array"}}},"published":false},"meta":{"code":200}}`
	if diff := cmp.Diff(expectBody, resultBody); diff != "" {
		t.Errorf("api response (-want +got):\n%s", diff)
	}

	// Body with no history
	actualStatusCode, actualBody = APICall("/body/peer/test_ds", dsHandler.BodyHandler)
	if actualStatusCode != 422 {
		t.Errorf("expected status code 422, got %d", actualStatusCode)
	}
	expectBody = `{ "meta": { "code": 422, "error": "no history" }, "data": null }`
	if expectBody != actualBody {
		t.Errorf("expected body %s, got %s", expectBody, actualBody)
	}

	// Body with no history, but FSI working directory has body
	actualStatusCode, actualBody = APICall("/body/peer/test_ds?fsi=true", dsHandler.BodyHandler)
	if actualStatusCode != 200 {
		t.Errorf("expected status code 200, got %d", actualStatusCode)
	}
	expectBody = `{"data":{"path":"","data":[["one","two",3],["four","five",6]]},"meta":{"code":200},"pagination":{"nextUrl":"/body/peer/test_ds?fsi=true\u0026page=2"}}`
	if expectBody != actualBody {
		t.Errorf("expected body %s, got %s", expectBody, actualBody)
	}

	fsiHandler := NewFSIHandlers(inst, false)

	// Status at version with no history
	actualStatusCode, actualBody = APICall("/status/peer/test_ds", fsiHandler.StatusHandler("/status"))
	if actualStatusCode != 422 {
		t.Errorf("expected status code 422, got %d", actualStatusCode)
	}
	expectBody = `{ "meta": { "code": 422, "error": "no history" }, "data": null }`
	if expectBody != actualBody {
		t.Errorf("expected body %s, got %s", expectBody, actualBody)
	}

	// Status with no history, but FSI working directory has contents
	actualStatusCode, actualBody = APICall("/status/peer/test_ds?fsi=true", fsiHandler.StatusHandler("/status"))
	if actualStatusCode != 200 {
		t.Errorf("expected status code 200, got %d", actualStatusCode)
	}
	// Handle tempoary directory by replacing the temp part with a shorter string.
	resultBody = strings.Replace(actualBody, initDir, initSubdir, -1)
	templateBody := `{"data":[{"sourceFile":"fsi_init_dir/meta.json","component":"meta","type":"add","message":"","mtime":"%s"},{"sourceFile":"fsi_init_dir/schema.json","component":"schema","type":"add","message":"","mtime":"%s"},{"sourceFile":"body.csv","component":"body","type":"add","message":"","mtime":"%s"}],"meta":{"code":200}}`
	expectBody = fmt.Sprintf(templateBody, metaMtime, schemaMtime, bodyMtime)
	if diff := cmp.Diff(expectBody, resultBody); diff != "" {
		t.Errorf("api response (-want +got):\n%s", diff)
	}

	logHandler := NewLogHandlers(node)

	// History with no history
	actualStatusCode, actualBody = APICall("/history/peer/test_ds", logHandler.LogHandler)
	if actualStatusCode != 422 {
		t.Errorf("expected status code 422, got %d", actualStatusCode)
	}
	expectBody = `{ "meta": { "code": 422, "error": "no history" }, "data": null }`
	if expectBody != actualBody {
		t.Errorf("expected body %s, got %s", expectBody, actualBody)
	}

	// History with no history, still returns ErrNoHistory since this route ignores fsi param
	actualStatusCode, actualBody = APICall("/history/peer/test_ds?fsi=true", logHandler.LogHandler)
	if actualStatusCode != 422 {
		t.Errorf("expected status code 422, got %d", actualStatusCode)
	}
	expectBody = `{ "meta": { "code": 422, "error": "no history" }, "data": null }`
	if expectBody != actualBody {
		t.Errorf("expected body %s, got %s", expectBody, actualBody)
	}
}

func TestCheckoutAndRestore(t *testing.T) {
	node, teardown := newTestNode(t)
	defer teardown()

	inst := newTestInstanceWithProfileFromNode(node)

	tmpDir := os.TempDir()
	workSubdir := "fsi_checkout_restore"
	workDir := filepath.Join(tmpDir, workSubdir)
	// Don't create the work directory, it must not exist for checkout to work. Remove if it
	// already exists.
	_ = os.RemoveAll(workDir)

	dr := lib.NewDatasetRequests(node, nil)

	// Save version 1
	saveParams := lib.SaveParams{
		Ref: "me/fsi_checkout_restore",
		Dataset: &dataset.Dataset{
			Meta: &dataset.Meta{
				Title: "title one",
			},
		},
		BodyPath: "testdata/cities/data.csv",
	}
	res := repo.DatasetRef{}
	if err := dr.Save(&saveParams, &res); err != nil {
		t.Fatal(err)
	}

	// Save the path from reference for later.
	// TODO(dlong): Support full dataset refs, not just the path.
	pos := strings.Index(res.String(), "/map/")
	ref1 := res.String()[pos:]

	// Save version 2 with a different title
	saveParams = lib.SaveParams{
		Ref: "me/fsi_checkout_restore",
		Dataset: &dataset.Dataset{
			Meta: &dataset.Meta{
				Title: "title two",
			},
		},
	}
	if err := dr.Save(&saveParams, &res); err != nil {
		t.Fatal(err)
	}

	fsiHandler := NewFSIHandlers(inst, false)

	// Checkout the dataset
	actualStatusCode, actualBody := APICallWithParams(
		"POST",
		"/checkout/peer/fsi_checkout_restore",
		map[string]string{
			"dir": workDir,
		},
		fsiHandler.CheckoutHandler("/checkout"))
	if actualStatusCode != 200 {
		t.Errorf("expected status code 200, got %d", actualStatusCode)
	}
	expectBody := `{"data":"","meta":{"code":200}}`
	if expectBody != actualBody {
		t.Errorf("expected body %s, got %s", expectBody, actualBody)
	}

	// Read meta.json, should have "title two" as the meta title
	metaContents, err := ioutil.ReadFile(filepath.Join(workDir, "meta.json"))
	if err != nil {
		t.Fatalf(err.Error())
	}
	expectContents := "{\n \"qri\": \"md:0\",\n \"title\": \"title two\"\n}"
	if diff := cmp.Diff(expectContents, string(metaContents)); diff != "" {
		t.Errorf("meta.json contents (-want +got):\n%s", diff)
	}

	// Overwrite meta so it has a different title
	if err = ioutil.WriteFile("meta.json", []byte(`{"title": "hello"}`), os.ModePerm); err != nil {
		t.Fatalf(err.Error())
	}

	// Restore the meta component
	actualStatusCode, actualBody = APICallWithParams(
		"POST",
		"/restore/peer/fsi_checkout_restore",
		map[string]string{
			"component": "meta",
		},
		fsiHandler.RestoreHandler("/restore"))
	if actualStatusCode != 200 {
		t.Errorf("expected status code 200, got %d", actualStatusCode)
	}
	expectBody = `{"data":"","meta":{"code":200}}`
	if expectBody != actualBody {
		t.Errorf("expected body %s, got %s", expectBody, actualBody)
	}

	// Read meta.json, should once again have "title two" as the meta title
	metaContents, err = ioutil.ReadFile(filepath.Join(workDir, "meta.json"))
	if err != nil {
		t.Fatalf(err.Error())
	}
	expectContents = "{\n \"qri\": \"md:0\",\n \"title\": \"title two\"\n}"
	if diff := cmp.Diff(expectContents, string(metaContents)); diff != "" {
		t.Errorf("meta.json contents (-want +got):\n%s", diff)
	}

	// Restore the previous version of the dataset
	actualStatusCode, actualBody = APICallWithParams(
		"POST",
		"/restore/peer/fsi_checkout_restore",
		map[string]string{
			// TODO(dlong): Have to pass "dir" to this method. In the test, the ref does
			// not have an FSIPath. Might be because we're using /map/, not sure.
			"dir":  workDir,
			"path": ref1,
		},
		fsiHandler.RestoreHandler("/restore"))
	if actualStatusCode != 200 {
		t.Errorf("expected status code 200, got %d", actualStatusCode)
	}
	expectBody = `{"data":"","meta":{"code":200}}`
	if expectBody != actualBody {
		t.Errorf("expected body %s, got %s", expectBody, actualBody)
	}

	// Read meta.json, should now have "title one" as the meta title
	metaContents, err = ioutil.ReadFile(filepath.Join(workDir, "meta.json"))
	if err != nil {
		t.Fatalf(err.Error())
	}
	expectContents = "{\n \"qri\": \"md:0\",\n \"title\": \"title one\"\n}"
	if diff := cmp.Diff(expectContents, string(metaContents)); diff != "" {
		t.Errorf("meta.json contents (-want +got):\n%s", diff)
	}
}

// APICall calls the api and returns the status code and body
func APICall(url string, hf http.HandlerFunc) (int, string) {
	return APICallWithParams("GET", url, nil, hf)
}

// APICallWithParams calls the api and returns the status code and body
func APICallWithParams(method, reqURL string, params map[string]string, hf http.HandlerFunc) (int, string) {
	// Add parameters from map
	reqParams := url.Values{}
	if params != nil {
		for key := range params {
			reqParams.Set(key, params[key])
		}
	}
	req := httptest.NewRequest(method, reqURL, strings.NewReader(reqParams.Encode()))
	// Set form-encoded header so server will find the parameters
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(reqParams.Encode())))
	w := httptest.NewRecorder()
	hf(w, req)
	res := w.Result()
	statusCode := res.StatusCode
	bodyBytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		panic(err)
	}
	return statusCode, string(bodyBytes)
}