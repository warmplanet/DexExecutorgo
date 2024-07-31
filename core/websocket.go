package core

import (
	"DexExecutorgo/utils"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type WsConfig struct {
	WsUrl                    string
	ProxyUrl                 string
	BindAddr                 string              // 绑定的本地ip地址，用于负载均衡
	ReqHeaders               map[string][]string //连接的时候加入的头部信息
	HeartbeatIntervalTime    time.Duration       //
	HeartbeatData            func() []byte       //心跳数据2
	HeartbeatDataStream      func() [][]byte     //心跳数据2
	IsAutoReconnect          bool
	ProtoHandleFunc          func([]byte) error           //协议处理函数
	DecompressFunc           func([]byte) ([]byte, error) //解压函数
	ErrorHandleFunc          func(err error)
	CustomSub                func() []byte
	PreReconnectHook         func(*WsConn, int) error // 重连前调用 改变URL等 参数: self, retry
	PostReconnectSuccessHook func(*WsConn) error      // 重连成功后调用 重新发订阅消息等
	IsDump                   bool
	DisableEnableCompression bool
	readDeadLineTime         time.Duration
	reconnectInterval        time.Duration
	errMsg                   string
	errSleepInterval         time.Duration
}

type WsConn struct {
	c *websocket.Conn
	WsConfig
	writeBufferChan        chan []byte
	pingMessageBufferChan  chan []byte
	pongMessageBufferChan  chan []byte
	closeMessageBufferChan chan []byte
	close                  chan bool
	reConnectLock          *sync.Mutex
}

type WsBuilder struct {
	wsConfig *WsConfig
}

func NewWsBuilderWithReadTimeout(readTimeout time.Duration) *WsBuilder {
	return &WsBuilder{&WsConfig{
		ReqHeaders:        make(map[string][]string, 1),
		reconnectInterval: time.Second * 10,
		readDeadLineTime:  readTimeout,
	}}
}

func NewWsBuilder() *WsBuilder {
	return &WsBuilder{&WsConfig{
		ReqHeaders:        make(map[string][]string, 1),
		reconnectInterval: time.Second * 10,
	}}
}

func (b *WsBuilder) WsUrl(wsUrl string) *WsBuilder {
	b.wsConfig.WsUrl = wsUrl
	return b
}

func (b *WsBuilder) ProxyUrl(proxyUrl string) *WsBuilder {
	b.wsConfig.ProxyUrl = proxyUrl
	return b
}

func (b *WsBuilder) BindAddr(bindAddr string) *WsBuilder {
	b.wsConfig.BindAddr = bindAddr
	return b
}

func (b *WsBuilder) ReqHeader(key, value string) *WsBuilder {
	b.wsConfig.ReqHeaders[key] = append(b.wsConfig.ReqHeaders[key], value)
	return b
}

func (b *WsBuilder) AutoReconnect() *WsBuilder {
	b.wsConfig.IsAutoReconnect = true
	return b
}

func (b *WsBuilder) Dump() *WsBuilder {
	b.wsConfig.IsDump = true
	return b
}

func (b *WsBuilder) Heartbeat(heartbeat func() []byte, t time.Duration) *WsBuilder {
	b.wsConfig.HeartbeatIntervalTime = t
	b.wsConfig.HeartbeatData = heartbeat
	return b
}

func (b *WsBuilder) SetError(errMsg string, t time.Duration) *WsBuilder {
	b.wsConfig.errMsg = errMsg
	b.wsConfig.errSleepInterval = t
	return b
}

func (b *WsBuilder) HeartbeatStream(heartbeat func() [][]byte, t time.Duration) *WsBuilder {
	b.wsConfig.HeartbeatIntervalTime = t
	b.wsConfig.HeartbeatDataStream = heartbeat
	return b
}

func (b *WsBuilder) ReconnectInterval(t time.Duration) *WsBuilder {
	b.wsConfig.reconnectInterval = t
	return b
}

func (b *WsBuilder) ProtoHandleFunc(f func([]byte) error) *WsBuilder {
	b.wsConfig.ProtoHandleFunc = f
	return b
}

func (b *WsBuilder) DisableEnableCompression() *WsBuilder {
	b.wsConfig.DisableEnableCompression = true
	return b
}

func (b *WsBuilder) DecompressFunc(f func([]byte) ([]byte, error)) *WsBuilder {
	b.wsConfig.DecompressFunc = f
	return b
}

func (b *WsBuilder) ErrorHandleFunc(f func(err error)) *WsBuilder {
	b.wsConfig.ErrorHandleFunc = f
	return b
}

func (b *WsBuilder) CustomSub(f func() []byte) *WsBuilder {
	b.wsConfig.CustomSub = f
	return b
}

func (b *WsBuilder) PostReconnectSuccess(hook func(*WsConn) error) *WsBuilder {
	b.wsConfig.PostReconnectSuccessHook = hook
	return b
}

func (b *WsBuilder) PreReconnect(hook func(*WsConn, int) error) *WsBuilder {
	b.wsConfig.PreReconnectHook = hook
	return b
}

func (b *WsBuilder) Build() *WsConn {
	wsConn := &WsConn{WsConfig: *b.wsConfig}
	return wsConn.NewWs()
}

func (ws *WsConn) NewWs() *WsConn {
	if ws.HeartbeatIntervalTime > ws.readDeadLineTime {
		ws.readDeadLineTime = ws.HeartbeatIntervalTime * 2
	}

	if err := ws.connect(); err != nil {
		fmt.Println(ws.WsUrl, err.Error())
		utils.Logger.Error(fmt.Errorf("[%s] %s", ws.WsUrl, err.Error()))
		return nil
	}

	ws.close = make(chan bool, 1)
	ws.pingMessageBufferChan = make(chan []byte, 10)
	ws.pongMessageBufferChan = make(chan []byte, 10)
	ws.closeMessageBufferChan = make(chan []byte, 10)
	ws.writeBufferChan = make(chan []byte, 10)
	ws.reConnectLock = new(sync.Mutex)

	go ws.writeRequest()
	go ws.receiveMessage()

	// if ws.ConnectSuccessAfterSendMessage != nil {
	// 	msg := ws.ConnectSuccessAfterSendMessage()
	// 	ws.SendMessage(msg)
	// 	logger.Logger.Infof("[ws] [%s] execute the connect success after send message=%s", ws.WsUrl, string(msg))
	// }

	return ws
}

func (ws *WsConn) connect() error {
	var dialer = &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 30 * time.Second,
	}

	if ws.BindAddr != "" {
		localAddr, _ := net.ResolveIPAddr("ip", ws.BindAddr)
		dialer.NetDialContext = (&net.Dialer{LocalAddr: &net.TCPAddr{IP: localAddr.IP}}).DialContext
	}

	if ws.ProxyUrl != "" {
		proxy, err := url.Parse(ws.ProxyUrl)
		if err == nil {
			utils.Logger.Infof("[ws][%s] proxy url:%s", ws.WsUrl, proxy)
			dialer.Proxy = http.ProxyURL(proxy)
		} else {
			utils.Logger.Errorf("[ws][%s]parse proxy url [%s] err %s  ", ws.WsUrl, ws.ProxyUrl, err.Error())
		}
	}

	if ws.DisableEnableCompression {
		dialer.EnableCompression = false
	}

	wsConn, resp, err := dialer.Dial(ws.WsUrl, http.Header(ws.ReqHeaders))
	if err != nil {
		utils.Logger.Errorf("[ws][%s] connect dialer err: %s", ws.WsUrl, err.Error())
		if ws.IsDump && resp != nil {
			dumpData, _ := httputil.DumpResponse(resp, true)
			utils.Logger.Debugf("[ws][%s] connect dump dialer error response err: %s", ws.WsUrl, string(dumpData))
		}
		return err
	}

	wsConn.SetReadDeadline(time.Now().Add(ws.readDeadLineTime))

	if ws.IsDump {
		dumpData, _ := httputil.DumpResponse(resp, true)
		utils.Logger.Debugf("[ws][%s] connect dump response err: %s", ws.WsUrl, string(dumpData))
	}
	utils.Logger.Infof("[ws][%s] connected", ws.WsUrl)
	ws.c = wsConn
	return nil
}

func (ws *WsConn) Close() {
	if isChanClose(ws.writeBufferChan) {
		return
	}
	ws.c.Close()
}

func (ws *WsConn) Reconnect() {
	ws.reConnectLock.Lock()
	defer ws.reConnectLock.Unlock()
	if isChanClose(ws.writeBufferChan) {
		return
	}
	ws.c.Close() //主动关闭一次
	var err error
	for retry := 1; retry <= 100; retry++ {
		if ws.PreReconnectHook != nil {
			err = ws.PreReconnectHook(ws, retry)
			if err != nil {
				utils.Logger.Errorf("reconnect hook(%d) fail , %s", retry, err.Error())
			}
		}
		err = ws.connect()
		if err != nil {
			utils.Logger.Errorf("[ws] [%s] websocket reconnect(%d) fail , %s", ws.WsUrl, retry, err.Error())
		} else {
			break
		}
		time.Sleep(ws.WsConfig.reconnectInterval * time.Duration(retry))
	}

	if err != nil {
		utils.Logger.Errorf("[ws] [%s] retry connect 100 count fail , begin exiting. ", ws.WsUrl)
		ws.CloseWs()
		if ws.ErrorHandleFunc != nil {
			ws.ErrorHandleFunc(errors.New("retry reconnect fail"))
		}
	} else if ws.CustomSub != nil {
		utils.Logger.Infof("连接断开，使用自定义机制开始重连")
		ws.SendMessage(ws.CustomSub())
	} else {
		//re subscribe
		// if ws.ConnectSuccessAfterSendMessage != nil {
		// 	msg := ws.ConnectSuccessAfterSendMessage()
		// 	ws.SendMessage(msg)
		// 	logger.Logger.Infof("[ws] [%s] execute the connect success after send message=%s", ws.WsUrl, string(msg))
		// 	time.Sleep(time.Second) //wait response
		// }
		if ws.PostReconnectSuccessHook != nil {
			err = ws.PostReconnectSuccessHook(ws)
			if err != nil {
				utils.Logger.Errorf("post reconnect hook fail , %s", err.Error())
			}
		}

		// for _, sub := range ws.subs {
		// 	fmt.Println("subscribe", string(sub))
		// 	logger.Logger.Info("[ws] re subscribe: ", string(sub))
		// 	ws.SendMessage(sub)
		// }
	}
}

func (ws *WsConn) writeRequest() {
	var (
		heartTimer *time.Timer
		err        error
	)

	if ws.HeartbeatIntervalTime == 0 {
		heartTimer = time.NewTimer(time.Hour)
	} else {
		heartTimer = time.NewTimer(ws.HeartbeatIntervalTime)
	}

	for {
		select {
		case <-ws.close:
			utils.Logger.Infof("[ws][%s] close websocket , exiting write message goroutine.", ws.WsUrl)
			return
		case d := <-ws.writeBufferChan:
			err = ws.c.WriteMessage(websocket.TextMessage, d)
		case d := <-ws.pingMessageBufferChan:
			err = ws.c.WriteMessage(websocket.PingMessage, d)
		case d := <-ws.pongMessageBufferChan:
			err = ws.c.WriteMessage(websocket.PongMessage, d)
		case d := <-ws.closeMessageBufferChan:
			err = ws.c.WriteMessage(websocket.CloseMessage, d)
		case <-heartTimer.C:
			if ws.HeartbeatIntervalTime > 0 {
				if ws.HeartbeatData != nil {
					err = ws.c.WriteMessage(websocket.TextMessage, ws.HeartbeatData())
				} else if ws.HeartbeatDataStream != nil {
					data := ws.HeartbeatDataStream()
					for _, i := range data {
						err = ws.c.WriteMessage(websocket.TextMessage, i)
					}
				}
				heartTimer.Reset(ws.HeartbeatIntervalTime)
			}
		}

		if err != nil {
			fmt.Printf("[ws][%s] write message , Begin Retry Connect: %s\n", ws.WsUrl, err.Error())
			utils.Logger.Errorf("[ws][%s] write message , Begin Retry Connect: %s", ws.WsUrl, err.Error())
			if ws.errMsg != "" && strings.Contains(err.Error(), ws.errMsg) {
				time.Sleep(ws.errSleepInterval)
			} else {
				time.Sleep(time.Second)
			}
			ws.Reconnect()
		}
	}
}

func (ws *WsConn) Subscribe(subEvent interface{}) error {
	data, err := json.Marshal(subEvent)
	utils.Logger.Infof("[ws][%s] trade send: %s", ws.WsUrl, string(data))
	if err != nil {
		utils.Logger.Errorf("[ws][%s] json encode error , %s", ws.WsUrl, err)
		return err
	}
	ws.writeBufferChan <- data
	// ws.subs = append(ws.subs, data)
	return nil
}

func (ws *WsConn) SendMessage(msg []byte) {
	ws.writeBufferChan <- msg
}

func (ws *WsConn) SendPingMessage(msg []byte) {
	ws.pingMessageBufferChan <- msg
}

func (ws *WsConn) SendPongMessage(msg []byte) {
	ws.pongMessageBufferChan <- msg
}

func (ws *WsConn) SendCloseMessage(msg []byte) {
	ws.closeMessageBufferChan <- msg
}

func (ws *WsConn) SendJsonMessage(m interface{}) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	ws.writeBufferChan <- data
	return nil
}

func (ws *WsConn) receiveMessage() {
	//rand.Seed(time.Now().UnixMicro())
	//exit
	ws.c.SetCloseHandler(func(code int, text string) error {
		utils.Logger.Warnf("[ws][%s] websocket exiting [code=%d , text=%s]", ws.WsUrl, code, text)
		//ws.CloseWs()
		return nil
	})

	ws.c.SetPongHandler(func(pong string) error {
		utils.Logger.Debugf("[%s] received [pong] %s", ws.WsUrl, pong)
		ws.c.SetReadDeadline(time.Now().Add(ws.readDeadLineTime))
		return nil
	})

	ws.c.SetPingHandler(func(ping string) error {
		utils.Logger.Debugf("[%s] received [ping] %s", ws.WsUrl, ping)
		ws.SendPongMessage([]byte(ping))
		ws.c.SetReadDeadline(time.Now().Add(ws.readDeadLineTime))
		return nil
	})

	for {
		select {
		case <-ws.close:
			utils.Logger.Infof("[ws][%s] close websocket , exiting receive message goroutine.", ws.WsUrl)
			return
		default:
			t, msg, err := ws.c.ReadMessage()
			//if rand.Intn(100) == 2 {
			//	err = errors.New("人为错误")
			//}
			if err != nil {
				utils.Logger.Errorf("[ws][%s] read message error: %s", ws.WsUrl, err.Error())
				if ws.IsAutoReconnect {
					fmt.Printf("[ws][%s] Unexpected Closed , Begin Retry Connect: %s\n", ws.WsUrl, err.Error())
					utils.Logger.Infof("[ws][%s] Unexpected Closed , Begin Retry Connect.", ws.WsUrl)
					ws.Reconnect()
					continue
				}

				if ws.ErrorHandleFunc != nil {
					ws.ErrorHandleFunc(err)
				}

				return
			}
			//			logger.Logger.Debug(string(msg))
			ws.c.SetReadDeadline(time.Now().Add(ws.readDeadLineTime))
			switch t {
			case websocket.TextMessage:
				err = ws.ProtoHandleFunc(msg)
				if err != nil {
					utils.Logger.Error(err)
				}
			case websocket.BinaryMessage:
				if ws.DecompressFunc == nil {
					ws.ProtoHandleFunc(msg)
				} else {
					msg2, err := ws.DecompressFunc(msg)
					if err != nil {
						utils.Logger.Errorf("[ws][%s] decompress error %s", ws.WsUrl, err.Error())
					} else {
						ws.ProtoHandleFunc(msg2)
					}
				}
				//	case websocket.CloseMessage:
				//	ws.CloseWs()
			default:
				utils.Logger.Errorf("[ws][%s] error websocket message type , content is :\n %s \n", ws.WsUrl, string(msg))
			}
		}
	}
}

func isChanClose[T any](ch chan T) bool {
	select {
	case _, received := <-ch:
		return !received
	default:
	}
	return false
}

func (ws *WsConn) CloseWs() {
	ws.reConnectLock.Lock()
	defer ws.reConnectLock.Unlock()

	if !isChanClose(ws.close) {
		select {
		case ws.close <- true:
		case <-time.After(1 * time.Second):
			utils.Logger.Errorf("[ws] close websocket send close channel time out")
		}
		close(ws.close)
		close(ws.writeBufferChan)
		close(ws.closeMessageBufferChan)
		close(ws.pingMessageBufferChan)
		close(ws.pongMessageBufferChan)
	}
	err := ws.c.Close()
	if err != nil {
		utils.Logger.Error("[ws][", ws.WsUrl, "] close websocket error ,", err)
	}
}
