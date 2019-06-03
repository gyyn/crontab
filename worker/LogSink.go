package worker

import (
	"context"
	"time"

	"gopkg.in/gomail.v2"

	"github.com/gyyn/crontab/common"
	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/mongodb/mongo-go-driver/mongo/clientopt"
)

//mongodb存储日志
type LogSink struct {
	client         *mongo.Client
	logCollection  *mongo.Collection
	logChan        chan *common.JobLog
	autoCommitChan chan *common.LogBatch
}

var (
	//单例
	G_logSink *LogSink
)

//批量写入日志
func (logSink *LogSink) saveLogs(batch *common.LogBatch) {
	logSink.logCollection.InsertMany(context.TODO(), batch.Logs)
}

//日志存储协程
func (logSink *LogSink) writeLoop() {
	var (
		log          *common.JobLog
		logBatch     *common.LogBatch //当前的批次
		commitTimer  *time.Timer
		timeoutBatch *common.LogBatch //超时批次
	)

	for {
		select {
		case log = <-logSink.logChan:

			if log.Err == "" {
				to := []string{log.Email}
				localIp, _ := GetLocalIP()

				subject := log.JobName + " " + time.Unix(time.Now().Unix(), 0).Format("2006-01-02 15:04:05") + " Err"

				strPlanTime := time.Unix(log.PlanTime/1000, 0).Format("2006-01-02 15:04:05")
				strScheduleTime := time.Unix(log.ScheduleTime/1000, 0).Format("2006-01-02 15:04:05")
				strStartTime := time.Unix(log.StartTime/1000, 0).Format("2006-01-02 15:04:05")
				strEndTime := time.Unix(log.EndTime/1000, 0).Format("2006-01-02 15:04:05")

				body := "Err\n" +
					"JobName: " + log.JobName + "\r\n" +
					"Command: " + log.Command + "\r\n" +
					"Output: " + log.Output + "\r\n" +
					"PlanTime: " + strPlanTime + "\r\n" +
					"ScheduleTime: " + strScheduleTime + "\r\n" +
					"StartTime: " + strStartTime + "\r\n" +
					"EndTime: " + strEndTime + "\r\n" +
					"LocalIP: " + localIp

				sendMail(to, subject, body)
			}

			if logBatch == nil {
				logBatch = &common.LogBatch{}
				//让这个批次超时自动提交(给1秒的时间）
				commitTimer = time.AfterFunc(
					time.Duration(G_config.JobLogCommitTimeout)*time.Millisecond,
					func(batch *common.LogBatch) func() {
						return func() {
							logSink.autoCommitChan <- batch
						}
					}(logBatch),
				)
			}

			//把新日志追加到批次中
			logBatch.Logs = append(logBatch.Logs, log)

			//如果批次满了, 就立即发送
			if len(logBatch.Logs) >= G_config.JobLogBatchSize {
				//发送日志
				logSink.saveLogs(logBatch)
				//清空logBatch
				logBatch = nil
				//取消定时器
				commitTimer.Stop()
			}
		case timeoutBatch = <-logSink.autoCommitChan: //过期的批次
			//判断过期批次是否仍旧是当前的批次
			if timeoutBatch != logBatch {
				continue //跳过已经被提交的批次
			}
			//把批次写入到mongo中
			logSink.saveLogs(timeoutBatch)
			//清空logBatch
			logBatch = nil
		}
	}
}

func InitLogSink() (err error) {
	var (
		client *mongo.Client
	)

	//建立mongodb连接
	if client, err = mongo.Connect(
		context.TODO(),
		G_config.MongodbUri,
		clientopt.ConnectTimeout(time.Duration(G_config.MongodbConnectTimeout)*time.Millisecond)); err != nil {
		return
	}

	//选择db和collection
	G_logSink = &LogSink{
		client:         client,
		logCollection:  client.Database("cron").Collection("log"),
		logChan:        make(chan *common.JobLog, 1000),
		autoCommitChan: make(chan *common.LogBatch, 1000),
	}

	//启动一个mongodb处理协程
	go G_logSink.writeLoop()
	return
}

//发送日志
func (logSink *LogSink) Append(jobLog *common.JobLog) {
	select {
	case logSink.logChan <- jobLog:
	default:
		//队列满了就丢弃
	}
}

// 发送邮件
func sendMail(to []string, subject string, body string) error {
	username := "gpcrontab@163.com"
	password := "crontab163"
	host := "smtp.163.com"
	//465 SMTPS
	port := 465
	from := "gpcrontab@163.com"
	send_to := common.StrSliceRemoveRepeat(to) // 去重
	m := gomail.NewMessage()
	m.SetAddressHeader("From", from, from) //发件人
	m.SetHeader("To", send_to...)          //收件人
	//m.SetAddressHeader("Cc", "dan@example.com", "Dan")  //抄送
	//m.SetHeader("Bcc", m.FormatAddress("xxxx@gmail.com", "xxxx")) //暗送
	m.SetHeader("Subject", subject) //主题
	m.SetBody("text/html", body)    //正文
	//m.Attach("/home/Alex/lolcat.jpg") //附件
	d := gomail.NewDialer(host, port, username, password)
	return d.DialAndSend(m)
}
