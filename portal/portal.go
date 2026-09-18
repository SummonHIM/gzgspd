package portal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/robertkrimen/otto"

	"github.com/summonhim/gzgspd/nnet"
)

// ActionResponse PortalJsonAction 返回的结构
type ActionResponse struct {
	AuthByRas    bool          `json:"authByRas"`
	DropMacAuth  bool          `json:"dropMacAuth"`
	GroupFeeList []interface{} `json:"groupFeeList"`
	IPType       int           `json:"ipType"`
	Local        interface{}   `json:"local"`
	MacChange    bool          `json:"macChange"`
	MicroAuth    struct {
		Type string `json:"type"`
	} `json:"microAuth"`
	Noticeconfig struct {
		NoticeList  []interface{} `json:"noticeList"`
		TemplateURL string        `json:"templateUrl"`
	} `json:"noticeconfig"`
	OAuthList []interface{} `json:"oAuthList"`
	OpenClass struct {
		ClassID     int    `json:"classId"`
		Description string `json:"description"`
	} `json:"openClass"`
	OperatingBindCtrlList []interface{} `json:"operatingBindCtrlList"`
	PortalForm            struct {
		Mac        string `json:"mac"`
		Vlan       string `json:"vlan"`
		Wlanacname string `json:"wlanacname"`
		Wlanuserip string `json:"wlanuserip"`
	} `json:"portalForm"`
	PortalConfig struct {
		AreaID                int    `json:"areaId"`
		Bmessage              string `json:"bmessage"`
		CheckOnlineFlag       int    `json:"checkOnlineFlag"`
		ClassID               int    `json:"classId"`
		Getpasstype           string `json:"getpasstype"`
		ID                    int    `json:"id"`
		List2Auth             string `json:"list2Auth"`
		ListOauthFlag         string `json:"listOauthFlag"`
		Listbindmac           string `json:"listbindmac"`
		Listfreeauth          string `json:"listfreeauth"`
		Listgetpass           string `json:"listgetpass"`
		Listpasscode          string `json:"listpasscode"`
		Listqqauth            string `json:"listqqauth"`
		Listwbauth            string `json:"listwbauth"`
		Listwxauth            string `json:"listwxauth"`
		Listwxmicroauth       string `json:"listwxmicroauth"`
		LogoutShowDetailFlag  string `json:"logoutShowDetailFlag"`
		Logoutgourl           string `json:"logoutgourl"`
		Logoutpageflag        string `json:"logoutpageflag"`
		ManagerID             string `json:"managerId"`
		Message1              string `json:"message1"`
		Message2              string `json:"message2"`
		Message3              string `json:"message3"`
		OperatorBindingPolicy string `json:"operatorBindingPolicy"`
		Passwd                string `json:"passwd"`
		PasswdCheckPolicy     string `json:"passwdCheckPolicy"`
		PayChangeGroupType    string `json:"payChangeGroupType"`
		Payflag               string `json:"payflag"`
		Picpath1              string `json:"picpath1"`
		Picpath2              string `json:"picpath2"`
		Picpath3              string `json:"picpath3"`
		Picpathurl1           string `json:"picpathurl1"`
		Picpathurl2           string `json:"picpathurl2"`
		Picpathurl3           string `json:"picpathurl3"`
		Portal2Pppoeflag      string `json:"portal2pppoeflag"`
		Sign                  string `json:"sign"`
		Smscontext            string `json:"smscontext"`
		Timestamp             int64  `json:"timestamp"`
		Title                 string `json:"title"`
		Tname                 string `json:"tname"`
		UserID                string `json:"userId"`
		Usertype              string `json:"usertype"`
		UUID                  string `json:"uuid"`
		Viewlogin             string `json:"viewlogin"`
	} `json:"portalconfig"`
	ServerForm struct {
		PortalVer  int    `json:"portalVer"`
		Serverip   string `json:"serverip"`
		Servername string `json:"servername"`
	} `json:"serverForm"`
}

// QuickAuthResponse QuickAuth 登录登出返回的结构
type QuickAuthResponse struct {
	Code                  string        `json:"code"`
	Rec                   string        `json:"rec"`
	Message               string        `json:"message"`
	WlanacIP              string        `json:"wlanacIp"`
	WlanacIpv6            string        `json:"wlanacIpv6"`
	Version               string        `json:"version"`
	Usertime              string        `json:"usertime"`
	Reccode               string        `json:"reccode"`
	Logoutgourl           string        `json:"logoutgourl"`
	SelfTicket            string        `json:"selfTicket"`
	MacChange             bool          `json:"macChange"`
	GroupID               int           `json:"groupId"`
	PasswdPolicyCheck     bool          `json:"passwdPolicyCheck"`
	DropLogCheck          string        `json:"dropLogCheck"`
	LogoutSsoURL          string        `json:"logoutSsoUrl"`
	UserID                string        `json:"userId"`
	OperatingBindCtrlList []interface{} `json:"operatingBindCtrlList"`
}

// Client 是绑定到某个本地 IP 的 Portal 协议客户端，可复用于多次请求。
type Client struct {
	http *http.Client
}

// NewClient 构造绑定本地 IP 的客户端。
func NewClient(localIP string, timeout time.Duration) (*Client, error) {
	hc, err := nnet.NewHttpClientBindIP(localIP, timeout)
	if err != nil {
		return nil, err
	}
	return &Client{http: hc}, nil
}

// Close 释放空闲连接。
func (c *Client) Close() {
	if c.http != nil {
		if tr, ok := c.http.Transport.(*http.Transport); ok {
			tr.CloseIdleConnections()
		}
	}
}

// doJSON 执行请求并将响应体解析到 out，非 2xx 视为错误。
func (c *Client) doJSON(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: unexpected status %d: %s", req.Method, req.URL, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s %s: decode response: %w", req.Method, req.URL, err)
	}
	return nil
}

func setCommonHeaders(req *http.Request, userAgent string) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7,ja;q=0.6")
}

// ActionRequest 是 PortalJsonAction 的请求参数。
type ActionRequest struct {
	Scheme     string
	Host       string
	UserAgent  string
	Wlanuserip string
	Wlanacname string
	MAC        string
	VLAN       string
	Hostname   string
	Rand       string
}

// PortalJsonAction 获取登录的基本信息
func (c *Client) PortalJsonAction(ctx context.Context, r ActionRequest) (*ActionResponse, error) {
	params := url.Values{}
	params.Set("wlanuserip", r.Wlanuserip)
	params.Set("wlanacname", r.Wlanacname)
	params.Set("mac", r.MAC)
	params.Set("vlan", r.VLAN)
	params.Set("hostname", r.Hostname)
	params.Set("rand", r.Rand)
	params.Set("viewStatus", "1")

	fullURL := r.Scheme + "://" + r.Host + "/PortalJsonAction.do?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	setCommonHeaders(req, r.UserAgent)

	var result ActionResponse
	if err := c.doJSON(req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// QuickAuthRequest 是 QuickAuth 的请求参数。
type QuickAuthRequest struct {
	Scheme       string
	Host         string
	UserAgent    string
	UserID       string
	Password     string
	Wlanuserip   string
	Wlanacname   string
	WlanacIP     string
	VLAN         string
	MAC          string
	Version      int
	PortalPageID int
	Timestamp    int64
	UUID         string
	PortalType   string
	Hostname     string
	Rand         string
}

// QuickAuth 执行 portal 登录。
func (c *Client) QuickAuth(ctx context.Context, r QuickAuthRequest) (*QuickAuthResponse, error) {
	params := url.Values{}
	params.Set("userid", r.UserID)
	params.Set("passwd", r.Password)
	params.Set("wlanuserip", r.Wlanuserip)
	params.Set("wlanacname", r.Wlanacname)
	params.Set("wlanacIp", r.WlanacIP)
	params.Set("vlan", r.VLAN)
	params.Set("mac", r.MAC)
	params.Set("version", fmt.Sprintf("%d", r.Version))
	params.Set("portalpageid", fmt.Sprintf("%d", r.PortalPageID))
	params.Set("timestamp", fmt.Sprintf("%d", r.Timestamp))
	params.Set("uuid", r.UUID)
	params.Set("portaltype", r.PortalType)
	params.Set("hostname", r.Hostname)
	params.Set("rand", r.Rand)

	fullURL := r.Scheme + "://" + r.Host + "/quickauth.do?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	setCommonHeaders(req, r.UserAgent)

	var result QuickAuthResponse
	if err := c.doJSON(req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DisconnRequest 是 QuickAuthDisconn 的请求参数。
type DisconnRequest struct {
	Scheme        string
	Host          string
	UserAgent     string
	WlanacIP      string
	Wlanuserip    string
	Wlanacname    string
	Version       int
	PortalType    string
	UserID        string
	MAC           string
	GroupID       int
	ClearOperator string
}

// QuickAuthDisconn 执行 portal 登出。
func (c *Client) QuickAuthDisconn(ctx context.Context, r DisconnRequest) (*QuickAuthResponse, error) {
	data := url.Values{}
	data.Set("wlanacip", r.WlanacIP)
	data.Set("wlanuserip", r.Wlanuserip)
	data.Set("wlanacname", r.Wlanacname)
	data.Set("version", fmt.Sprintf("%d", r.Version))
	data.Set("portaltype", r.PortalType)
	data.Set("userid", r.UserID)
	data.Set("mac", r.MAC)
	data.Set("groupId", fmt.Sprintf("%d", r.GroupID))
	data.Set("clearOperator", r.ClearOperator)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.Scheme+"://"+r.Host+"/quickauthdisconn.do", bytes.NewBufferString(data.Encode()))
	if err != nil {
		return nil, err
	}
	setCommonHeaders(req, r.UserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var result QuickAuthResponse
	if err := c.doJSON(req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// PortalChecker 探测当前网络是否需要登录。返回错误表示探测本身失败，而非"在线"。
func (c *Client) PortalChecker(ctx context.Context, kAliveLink string) (bool, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, kAliveLink, nil)
	if err != nil {
		return false, "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()

	portalKeywords := []string{"portalScript.do", "portal.do"}

	// 1) 检测 3xx 重定向
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		if loc, err := resp.Location(); err == nil {
			for _, kw := range portalKeywords {
				if strings.Contains(loc.String(), kw) {
					return true, loc.String(), nil
				}
			}
		}
	}

	// 2) 检测 200 页面内脚本的 location.replace
	if resp.StatusCode == 200 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return false, "", err
		}
		html := string(body)

		re := regexp.MustCompile(`<script[^>]*>([\s\S]*?)</script>`)
		scripts := re.FindAllStringSubmatch(html, -1)
		for _, s := range scripts {
			js := s[1]

			vm := otto.New()
			var finalURL string
			_ = vm.Set("location", map[string]interface{}{
				"replace": func(call otto.FunctionCall) otto.Value {
					v, _ := call.Argument(0).ToString()
					finalURL = v
					return otto.Value{}
				},
			})

			if _, err := vm.Run(js); err == nil && finalURL != "" {
				for _, kw := range portalKeywords {
					if strings.Contains(finalURL, kw) {
						return true, finalURL, nil
					}
				}
			}
		}
	}

	return false, "", nil
}
