package consulTool

import (
	"bytes"
	"fmt"
	"io"
	"k8s.io/apimachinery/pkg/util/json"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type Service struct {
	host       string
	port       int
	HttpClient *http.Client
}

func (s Service) Do(method string, path string, queries map[string]string, headers map[string]string, body io.Reader) (*http.Response, error) {
	u := url.URL{}
	u.Scheme = "http"
	u.Host = fmt.Sprintf("%s:%d", s.host, s.port)
	u.Path = path

	if queries != nil && len(queries) > 0 {
		values := url.Values{}
		for k, v := range queries {
			values.Add(k, v)
		}
		u.RawQuery = values.Encode()
	}
	req, er := http.NewRequest(method, u.String(), body)
	if er != nil {
		panic(er)
	}
	if headers != nil && len(headers) > 0 {
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}
	return s.HttpClient.Do(req)
}
func (s Service) Get(path string, headers map[string]string) (*http.Response, error) {
	return s.Do(http.MethodGet, path, nil, headers, nil)
}
func (s Service) PostJson(path string, headers map[string]string, data interface{}) (*http.Response, error) {
	bbytes, er := json.Marshal(data)
	if er != nil {
		panic(er)
	}
	body := &bytes.Buffer{}
	body.Write(bbytes)
	return s.Do(http.MethodPost, path, nil, headers, body)
}
func (s Service) PostForm(path string, headers map[string]string, data url.Values) (*http.Response, error) {
	return s.Do(http.MethodPost, path, nil, headers, strings.NewReader(data.Encode()))
}
func (s Service) PostMultipartForm(path string, headers map[string]string, files map[string][]os.File, form url.Values) (*http.Response, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if files == nil || len(files) == 0 {
		panic("files not be nil or empty")
	}
	for k, v := range files {
		if v != nil && len(v) > 0 {
			for _, f := range v {
				fw, er := writer.CreateFormFile(k, f.Name())
				if er != nil {
					panic(er)
				}
				_, err := io.Copy(fw, &f)
				if err != nil {
					panic(err)
				}
			}
		}
	}
	if form != nil && len(form) > 0 {
		for k, v := range form {
			if v != nil && len(v) > 0 {
				for _, val := range v {
					writer.WriteField(k, val)
				}
			}
		}
	}
	err := writer.Close() // close writer before POST request
	if err != nil {
		return nil, fmt.Errorf("writerClose: %v", err)
	}
	return s.Do(http.MethodPost, path, nil, headers, body)
}
