package resources

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/http/httputil"
	"net/url"
	"strings"
)

func AuthChe(oauthHost, masterUrl, host string, username string, password string) (*http.Client, error) {
	// Create the http client with a cookie jar
	j, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to initalize the cookie jar: %s", err)
	}

	transport := http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	client := &http.Client{
		Jar:           j,
		Transport:     &transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return nil },
	}

	_, err = DoAuthOpenshiftUserClient(client, oauthHost, masterUrl, "testing-idp", username, password)
	if err != nil {
		return nil, err
	}

	// Start the authentication
	// u := fmt.Sprintf("%s", host)
	u := "https://keycloak-edge-redhat-rhmi-rhsso.apps.cluster-pbraun-6554.pbraun-6554.example.opentlc.com/auth/realms/openshift/protocol/openid-connect/auth?client_id=che-client&redirect_uri=https://codeready-redhat-rhmi-codeready-workspaces.apps.cluster-pbraun-6554.pbraun-6554.example.opentlc.com/dashboard/&response_type=code&scope=openid"
	response, err := client.Get(u)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %s", u, err)
	}
	if response.StatusCode != 200 {
		return nil, errorWithResponseDump(response, fmt.Errorf("the request to %s failed with code %d", u, response.StatusCode))
	}

	// Select the testing IDP
	document, err := parseResponse(response)
	if err != nil {
		return nil, errorWithResponseDump(response, err)
	}

	s := dumpResponse(response)
	logrus.Info(s)

	// find the link to the testing IDP
	link, err := findElement(document, fmt.Sprintf("a:contains('%s')", testingIDP))
	if err != nil {
		return nil, errorWithResponseDump(response, err)
	}

	// get the url from the
	href, err := getAttribute(link, "href")
	if err != nil {
		return nil, errorWithResponseDump(response, err)
	}

	u, err = resolveRelativeURL(response, href)
	if err != nil {
		return nil, err
	}

	response, err = client.Get(u)
	s = dumpResponse(response)

	if err != nil {
		return nil, fmt.Errorf("failed to request %s: %s", u, err)
	}
	if response.StatusCode != 200 {
		return nil, errorWithResponseDump(response, fmt.Errorf("the request to %s failed with code %d", u, response.StatusCode))
	}

	code := strings.Split(response.Request.URL.RawQuery, "&")[1]


	tokenUrl := "https://keycloak-edge-redhat-rhmi-rhsso.apps.cluster-pbraun-6554.pbraun-6554.example.opentlc.com/auth/realms/openshift/protocol/openid-connect/token"

	formValues := url.Values{
		"grant_type": []string{"authorization_code"},
		"code": []string{strings.Split(code, "=")[1]},
		"client_id": []string{"che-client"},
		"redirect_uri": []string{"https://codeready-redhat-rhmi-codeready-workspaces.apps.cluster-pbraun-6554.pbraun-6554.example.opentlc.com/dashboard/"},
	}

	response, err = client.PostForm(tokenUrl, formValues)
	if err != nil {
		return nil, fmt.Errorf("failed to request %s: %s", u, err)
	}
	s = dumpResponse(response)

	// Submit the username and password
	document, err = parseResponse(response)
	if err != nil {
		return nil, errorWithResponseDump(response, err)
	}

	workspacesUrl := fmt.Sprintf("%v/api/workspace?skipCount=0&maxItems=30", host)
	response, err = client.Get(workspacesUrl)
	if err != nil {
		return nil, err
	}

	s = dumpResponse(response)

	// find the form for the login
	form, err := findElement(document, "#kc-form-login")
	if err != nil {
		return nil, errorWithResponseDump(response, err)
	}

	// retrieve the action of the form
	action, err := getAttribute(form, "action")
	if err != nil {
		return nil, errorWithResponseDump(response, err)
	}

	u, err = resolveRelativeURL(response, action)
	if err != nil {
		return nil, err
	}

	// submit the form with the username and password
	v := url.Values{"username": []string{username}, "password": []string{password}}
	response, err = client.PostForm(u, v)
	if err != nil {
		return nil, fmt.Errorf("failed to request %s: %s", u, err)
	}
	if response.StatusCode != 200 {
		return nil, errorWithResponseDump(response, fmt.Errorf("the request to %s failed with code %d", u, response.StatusCode))
	}

	document, err = parseResponse(response)
	if err != nil {
		return nil, err
	}

	// find the form for the approval
	form = document.Find("form")
	if err != nil {
		return nil, errorWithResponseDump(response, err)
	}

	// No form found: no further approval required, we are authenticated
	// at this point
	if form.Length() == 0 {
		return client, nil
	}

	// On first login the user is presented with an approval form. We have to submit
	// the form along with the scopes that we want to grant.
	_, err = approvePermissions(form, client, response)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// Login a user through the oauth proxy
func ProxyOAuth(host string, username string, password string) (*http.Client, error) {
	// Create the http client with a cookie jar
	j, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to initalize the cookie jar: %s", err)
	}

	transport := http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	client := &http.Client{
		Jar:           j,
		Transport:     &transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return nil },
	}

	// Start the authentication
	u := fmt.Sprintf("%s/oauth/start", host)
	response, err := client.Get(u)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %s", u, err)
	}
	if response.StatusCode != 200 {
		return nil, errorWithResponseDump(response, fmt.Errorf("the request to %s failed with code %d", u, response.StatusCode))
	}

	// Select the testing IDP
	document, err := parseResponse(response)
	if err != nil {
		return nil, errorWithResponseDump(response, err)
	}

	// find the link to the testing IDP
	link, err := findElement(document, fmt.Sprintf("a:contains('%s')", testingIDP))
	if err != nil {
		return nil, errorWithResponseDump(response, err)
	}

	// get the url from the
	href, err := getAttribute(link, "href")
	if err != nil {
		return nil, errorWithResponseDump(response, err)
	}

	u, err = resolveRelativeURL(response, href)
	if err != nil {
		return nil, err
	}

	response, err = client.Get(u)
	if err != nil {
		return nil, fmt.Errorf("failed to request %s: %s", u, err)
	}
	if response.StatusCode != 200 {
		return nil, errorWithResponseDump(response, fmt.Errorf("the request to %s failed with code %d", u, response.StatusCode))
	}

	// Submit the username and password
	document, err = parseResponse(response)
	if err != nil {
		return nil, errorWithResponseDump(response, err)
	}

	// find the form for the login
	form, err := findElement(document, "#kc-form-login")
	if err != nil {
		return nil, errorWithResponseDump(response, err)
	}

	// retrieve the action of the form
	action, err := getAttribute(form, "action")
	if err != nil {
		return nil, errorWithResponseDump(response, err)
	}

	u, err = resolveRelativeURL(response, action)
	if err != nil {
		return nil, err
	}

	// submit the form with the username and password
	v := url.Values{"username": []string{username}, "password": []string{password}}
	response, err = client.PostForm(u, v)
	if err != nil {
		return nil, fmt.Errorf("failed to request %s: %s", u, err)
	}
	if response.StatusCode != 200 {
		return nil, errorWithResponseDump(response, fmt.Errorf("the request to %s failed with code %d", u, response.StatusCode))
	}

	document, err = parseResponse(response)
	if err != nil {
		return nil, err
	}

	// find the form for the approval
	form = document.Find("form")
	if err != nil {
		return nil, errorWithResponseDump(response, err)
	}

	// No form found: no further approval required, we are authenticated
	// at this point
	if form.Length() == 0 {
		return client, nil
	}

	// On first login the user is presented with an approval form. We have to submit
	// the form along with the scopes that we want to grant.
	_, err = approvePermissions(form, client, response)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func verifyRedirect(redirectUrl, host string) error {
	if redirectUrl != host {
		return errors.New(fmt.Sprintf("redirect host does not match product host: %v / %v",
			redirectUrl,
			host))
	}
	return nil
}

// Submit permission approval form
func approvePermissions(form *goquery.Selection, client *http.Client, response *http.Response) (string, error) {
	// retrieve the action of the form
	action, err := getAttribute(form, "action")
	if err != nil {
		return "", err
	}

	// form submit url
	formUrl, err := resolveRelativeURL(response, action)
	if err != nil {
		return "", err
	}

	then, _ := form.Find("input[name='then']").Attr("value")
	csrf, _ := form.Find("input[name='csrf']").Attr("value")
	clientId, _ := form.Find("input[name='client_id']").Attr("value")
	userName, _ := form.Find("input[name='user_name']").Attr("value")
	redirectUrl, _ := form.Find("input[name='redirect_uri']").Attr("value")

	_, err = client.PostForm(formUrl, url.Values{
		"then":         []string{then},
		"csrf":         []string{csrf},
		"client_id":    []string{clientId},
		"user_name":    []string{userName},
		"redirect_uri": []string{redirectUrl},
		"scope":        []string{"user:info", "user:check-access"},
		"approve":      []string{"Allow+selected+permissions"},
	})
	return redirectUrl, err
}

func dumpResponse(r *http.Response) string {
	msg := "> Request\n"
	bytes, err := httputil.DumpRequestOut(r.Request, false)
	if err != nil {
		msg += fmt.Sprintf("failed to dump the request: %s", err)
	} else {
		msg += string(bytes)
	}
	msg += "\n"

	msg += "< Response\n"
	bytes, err = httputil.DumpResponse(r, true)
	if err != nil {
		msg += fmt.Sprintf("failed to dump the response: %s", err)
	} else {
		msg += string(bytes)
	}
	msg += "\n"

	return msg
}

func errorWithResponseDump(r *http.Response, err error) error {
	return fmt.Errorf("%s\n\n%s", err, dumpResponse(r))
}

func parseResponse(r *http.Response) (*goquery.Document, error) {
	// Clone the body while reading it so that in case of errors
	// we can dump the response with the body
	var clone bytes.Buffer
	body := io.TeeReader(r.Body, &clone)
	r.Body = ioutil.NopCloser(&clone)

	d, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, fmt.Errorf("failed to create the document: %s", err)
	}

	// <noscript> bug workaround
	// https://github.com/PuerkitoBio/goquery/issues/139#issuecomment-517526070
	d.Find("noscript").Each(func(i int, s *goquery.Selection) {
		s.SetHtml(s.Text())
	})

	return d, nil
}

func resolveRelativeURL(r *http.Response, relativeURL string) (string, error) {
	u, err := url.Parse(relativeURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse the url %s: %s", relativeURL, err)
	}

	u = r.Request.URL.ResolveReference(u)

	return u.String(), nil
}

func findElement(d *goquery.Document, selector string) (*goquery.Selection, error) {
	e := d.Find(selector)
	if e.Length() == 0 {
		return nil, fmt.Errorf("failed to find an element matching the selector %s", selector)
	}
	if e.Length() > 1 {
		return nil, fmt.Errorf("multiple element founded matching the selector %s", selector)
	}

	return e, nil
}

func getAttribute(element *goquery.Selection, name string) (string, error) {
	v, ok := element.Attr(name)
	if !ok {
		e, err := element.Html()
		if err != nil {
			e = fmt.Sprintf("failed to get the html content: %s", err)
		}

		return "", fmt.Errorf("the element '%s' doesn't have the %s attribute", e, name)
	}
	return v, nil
}
