package src

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
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

// PortalJsonAction 获取登录的基本信息
func PortalJsonAction(
	ifname string,
	host string,
	user_agent string,
	wlanuserip string,
	wlanacname string,
	mac string,
	vlan string,
	hostname string,
	randStr string,
) (*ActionResponse, error) {
	// 构造 URL 参数
	params := url.Values{}
	params.Set("wlanuserip", wlanuserip)
	params.Set("wlanacname", wlanacname)
	params.Set("mac", mac)
	params.Set("vlan", vlan)
	params.Set("hostname", hostname)
	params.Set("rand", randStr)
	params.Set("viewStatus", "1")

	fullURL := "http://" + host + "/PortalJsonAction.do?" + params.Encode()

	// 创建请求
	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, err
	}

	// 设置请求头
	req.Header.Set("User-Agent", user_agent)
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Accept-Language", "zh-CN")

	// 发起请求
	client := NewHttpClientWithIface(ifname, 5*time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 解析 JSON 到 struct
	var result ActionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// QuickAuth 登录
func QuickAuth(
	ifname string,
	host string,
	user_agent string,
	userid string,
	passwd string,
	wlanuserip string,
	wlanacname string,
	wlanacIp string,
	vlan string,
	mac string,
	version int,
	portalpageid int,
	timestamp int64,
	uuid string,
	portaltype string,
	hostname string,
	rand string,
) (*QuickAuthResponse, error) {
	// 构造 URL 参数
	params := url.Values{}
	params.Set("userid", userid)
	params.Set("passwd", passwd)
	params.Set("wlanuserip", wlanuserip)
	params.Set("wlanacname", wlanacname)
	params.Set("wlanacIp", wlanacIp)
	params.Set("vlan", vlan)
	params.Set("mac", mac)
	params.Set("version", fmt.Sprintf("%d", version))
	params.Set("portalpageid", fmt.Sprintf("%d", portalpageid))
	params.Set("timestamp", fmt.Sprintf("%d", timestamp))
	params.Set("uuid", uuid)
	params.Set("portaltype", portaltype)
	params.Set("hostname", hostname)
	params.Set("rand", rand)

	fullURL := "http://" + host + "/quickauth.do?" + params.Encode()

	// 创建请求
	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, err
	}

	// 设置请求头
	req.Header.Set("User-Agent", user_agent)
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7,ja;q=0.6")

	// 发起请求
	client := NewHttpClientWithIface(ifname, 5*time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 解析 JSON
	var result QuickAuthResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// QuickAuthDisconn 登出
func QuickAuthDisconn(
	ifname string,
	host string,
	user_agent string,
	wlanacip string,
	wlanuserip string,
	wlanacname string,
	version int,
	portaltype string,
	userid string,
	mac string,
	groupId int,
	clearOperator string,
) (*QuickAuthResponse, error) {
	// 构造表单数据
	data := url.Values{}
	data.Set("wlanacip", wlanacip)
	data.Set("wlanuserip", wlanuserip)
	data.Set("wlanacname", wlanacname)
	data.Set("version", fmt.Sprintf("%d", version))
	data.Set("portaltype", portaltype)
	data.Set("userid", userid)
	data.Set("mac", mac)
	data.Set("groupId", fmt.Sprintf("%d", groupId))
	data.Set("clearOperator", clearOperator)

	// 创建 POST 请求
	req, err := http.NewRequest("POST", "http://"+host+"/quickauthdisconn.do", bytes.NewBufferString(data.Encode()))
	if err != nil {
		return nil, err
	}

	// 设置请求头
	req.Header.Set("User-Agent", user_agent)
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7,ja;q=0.6")

	// 发起请求
	client := NewHttpClientWithIface(ifname, 5*time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 解析 JSON
	var result QuickAuthResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// PortalChecker 检查当前网络是否需要登录，若为是则返回登录链接
func PortalChecker(ifname string, kAliveLink string) (bool, string) {
	// 构造请求参数
	client := NewHttpClientWithIface(ifname, 5*time.Second)
	// 发起请求
	resp, err := client.Get(kAliveLink)

	// 请求结果分析
	if err != nil {
		return false, ""
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		loc, err := resp.Location()
		if err == nil && strings.Contains(loc.String(), "portalScript.do") {
			return true, loc.String()
		}
	}
	return false, ""
}
