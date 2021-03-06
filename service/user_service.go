package service

import (
	"bilibili/dao"
	"bilibili/model"
	"bilibili/tool"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/dysmsapi"
	"github.com/gin-gonic/gin"
	"math/rand"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type UserService struct {
}

func (u *UserService) SolveFollow(flag bool, followerUid int64, followingUid int64) error {
	d := dao.UserDao{tool.GetDb()}
	var err error

	if flag == false {
		//此前未关注
		err = d.InsertFollow(followerUid, followingUid)
		if err != nil {
			return err
		}

		//被关注用户更新关注者数量
		err = d.UpdateFollower(followingUid, 1)
		if err != nil {
			return err
		}

		//更新关注中数量
		err = d.UpdateFollowing(followerUid, 1)
		if err != nil {
			return err
		}

	} else {
		//此前已关注
		err = d.DeleteFollow(followerUid, followingUid)
		if err != nil {
			return err
		}

		//被关注用户更新关注者数量
		err = d.UpdateFollower(followingUid, -1)
		if err != nil {
			return err
		}

		//更新关注中数量
		err = d.UpdateFollowing(followerUid, -1)
		if err != nil {
			return err
		}
	}
	return nil
}

func (u *UserService) AddTotalView(uid int64) error {
	d := dao.UserDao{tool.GetDb()}

	err := d.UpdateTotalView(uid)
	return err
}

func (u *UserService) GetFollowStatus(followingUid, followedUid int64) (bool, error) {
	d := dao.UserDao{tool.GetDb()}

	followedUidSlice, err := d.QueryFollowedUid(followingUid)
	if err != nil {
		return false, err
	}

	for _, uid := range followedUidSlice {
		if followedUid == uid {
			return true, nil
		}
	}

	return false, nil
}

//获取每日任务的完成状态，依次返回签到状态，观看视频状态，投币获得的经验数量
func (u *UserService) GetDailyFlag(uid int64) (bool, bool, int64, error) {
	d := dao.UserDao{tool.GetDb()}
	timeNow := time.Now().Format("2006-01-02")
	var lastViewFlag, lastCheckInFlag, lastCoinFlag bool
	var dailyCoinNum int64

	userinfo, err := d.QueryByUid(uid)
	if err != nil {
		return false, false, 0, err
	}

	if userinfo.LastViewDate[:10] == timeNow {
		lastViewFlag = true
	} else {
		lastViewFlag = false
	}

	if userinfo.LastCheckInDate[:10] == timeNow {
		lastCheckInFlag = true
	} else {
		lastCheckInFlag = false
	}

	if userinfo.LastCoinDate[:10] == timeNow {
		lastCoinFlag = true
	} else {
		lastCoinFlag = false
	}

	if lastCoinFlag == true {
		//今日已经投币
		dailyCoinNum = userinfo.DailyCoin * 10
	} else {
		dailyCoinNum = 0
	}

	return lastCheckInFlag, lastViewFlag, dailyCoinNum, nil
}

//检查uid是否存在，存在返回true，反之返回false
func (u *UserService) JudgeUid(uid int64) (bool, error) {
	d := dao.UserDao{tool.GetDb()}

	_, err := d.QueryByUid(uid)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func (u *UserService) GetUidByEmail(email string) (int64, error) {
	d := dao.UserDao{tool.GetDb()}

	userinfo, err := d.QueryByEmail(email)
	if err != nil {
		return 0, err
	}

	return userinfo.Uid, nil
}

func (u *UserService) GetUidByPhone(phone string) (int64, error) {
	d := dao.UserDao{tool.GetDb()}

	userinfo, err := d.QueryByPhone(phone)
	if err != nil {
		return 0, err
	}

	return userinfo.Uid, nil
}

func (u *UserService) ChangePassword(uid int64, newPassword string) error {
	d := dao.UserDao{tool.GetDb()}

	//加盐
	salt := strconv.FormatInt(time.Now().Unix(), 10)
	m5 := md5.New()
	m5.Write([]byte(newPassword))
	m5.Write([]byte(salt))
	st := m5.Sum(nil)
	saltedPassword := hex.EncodeToString(st)

	err := d.UpdatePassword(uid, saltedPassword, salt)
	return err
}

func (u *UserService) ChangeUsername(uid int64, newUsername string) error {
	d := dao.UserDao{tool.GetDb()}

	err := d.UpdateUsername(uid, newUsername)
	if err != nil {
		return err
	}

	err = d.UpdateCoins(uid, -6)
	return err
}

func (u *UserService) GetSpaceUserinfo(uid int64) (model.SpaceUserinfo, error) {
	d := dao.UserDao{tool.GetDb()}
	vd := dao.VideoDao{tool.GetDb()}

	//获取基本信息
	spaceUserinfo, err := d.QuerySpaceUserinfoByUid(uid)
	if err != nil {
		return spaceUserinfo, err
	}

	//获取收藏夹信息
	var savesVideoSlice []model.Video
	savesAvSlice, err := vd.QuerySaveByUid(uid)
	if err != nil {
		return spaceUserinfo, err
	}

	for _, av := range savesAvSlice {
		videoModel, err := vd.QueryByAv(av)
		if err != nil {
			return spaceUserinfo, err
		}

		savesVideoSlice = append(savesVideoSlice, videoModel)
	}

	spaceUserinfo.Saves = savesVideoSlice
	//获取投稿信息
	postedVideoSlice, err := vd.QueryPostedVideoModelByAuthorUid(uid)
	if err != nil {
		return spaceUserinfo, err
	}

	spaceUserinfo.Videos = postedVideoSlice

	return spaceUserinfo, nil
}

func (u *UserService) GetUserinfo(uid int64) (model.Userinfo, error) {
	d := dao.UserDao{tool.GetDb()}

	userinfo, err := d.QueryByUid(uid)
	return userinfo, err
}

func (u *UserService) SolveViewExp(uid int64) (bool, error) {
	d := dao.UserDao{tool.GetDb()}

	userinfo, err := d.QueryByUid(uid)
	if err != nil {
		return false, err
	}

	lastViewDate := userinfo.LastViewDate[:10]
	timeNow := time.Now().Format("2006-01-02")

	if timeNow == lastViewDate {
		//已获得过经验
		return false, nil
	} else {
		//更新经验
		err = d.UpdateExp(uid, 5)
		if err != nil {
			return false, err
		}

		//更新记录
		err = d.UpdateLastViewDate(uid)
		if err != nil {
			return false, err
		}

		return true, nil
	}
}

//签到服务
func (u *UserService) CheckIn(uid int64) error {
	d := dao.UserDao{tool.GetDb()}

	//加经验
	err := d.UpdateExp(uid, 5)
	if err != nil {
		return err
	}
	//加硬币
	err = d.UpdateCoins(uid, 1)
	if err != nil {
		return err
	}
	//更新日期
	err = d.UpdateLastCheckInDate(uid)
	return err
}

//可以签到返回true，否则返回false
func (u *UserService) JudgeCheckIn(uid int64) (bool, error) {
	d := dao.UserDao{tool.GetDb()}

	userinfo, err := d.QueryByUid(uid)
	if err != nil {
		return false, err
	}

	lastCheckInDate := userinfo.LastCheckInDate[:10]
	timeNow := time.Now().Format("2006-01-02")

	if timeNow == lastCheckInDate {
		return false, nil
	}

	return true, nil
}

func (u *UserService) ChangeStatement(uid int64, newStatement string) error {
	d := dao.UserDao{tool.GetDb()}

	err := d.UpdateStatement(uid, newStatement)
	return err
}

func (u *UserService) SendCodeByEmail(email string) (string, error) {
	email = strings.ToLower(email)
	emailCfg := tool.GetCfg().Email

	auth := smtp.PlainAuth("", emailCfg.ServiceEmail, emailCfg.ServicePwd, emailCfg.SmtpHost)
	to := []string{email}

	fmt.Println("EMAIL", email)

	rand.Seed(time.Now().Unix())
	code := fmt.Sprintf("%06v", rand.New(rand.NewSource(time.Now().UnixNano())).Int31n(1000000))
	str := fmt.Sprintf("From:%v\r\nTo:%v\r\nSubject:bilibili验证码\r\n\r\n您的验证码为：%s\r\n请在10分钟内完成验证", emailCfg.ServiceEmail, email, code)
	msg := []byte(str)
	err := smtp.SendMail(emailCfg.SmtpHost+":"+emailCfg.SmtpPort, auth, emailCfg.ServiceEmail, to, msg)
	if err != nil {
		return "", err
	}

	return code, nil
}

func (u *UserService) ChangeBirthday(uid int64, newBirth time.Time) error {
	d := dao.UserDao{tool.GetDb()}

	err := d.UpdateBirthday(uid, newBirth)
	return err
}

func (u *UserService) ChangeGender(uid int64, newGender string) error {
	d := dao.UserDao{tool.GetDb()}

	err := d.UpdateGender(uid, newGender)
	return err
}

func (u *UserService) ChangeAvatar(uid int64, url string) error {
	d := dao.UserDao{tool.GetDb()}

	err := d.UpdateAvatar(uid, url)
	return err
}

func (u *UserService) ChangePhone(uid int64, newEmail string) error {
	d := dao.UserDao{tool.GetDb()}
	newEmail = strings.ToLower(newEmail)

	err := d.UpdatePhone(uid, newEmail)
	return err
}

func (u *UserService) ChangeEmail(uid int64, newEmail string) error {
	d := dao.UserDao{tool.GetDb()}
	newEmail = strings.ToLower(newEmail)

	err := d.UpdateEmail(uid, newEmail)
	return err
}

//通过短信登录
func (u *UserService) LoginBySms(phone string) (model.Userinfo, error) {
	d := dao.UserDao{tool.GetDb()}

	userinfo, err := d.QueryByPhone(phone)
	return userinfo, err
}

//通过密码登录，返回一个实体
func (u *UserService) Login(loginName, password string) (model.Userinfo, bool, error) {
	d := dao.UserDao{tool.GetDb()}

	//判断登录类型
	flag := strings.Index(loginName, "@")
	if flag != -1 {
		//邮箱登录
		loginName = strings.ToLower(loginName)
		userinfo, err := d.QueryByEmail(loginName)
		if err != nil {
			if err.Error() == "sql: no rows in result set" {
				return model.Userinfo{}, false, nil
			}
			return model.Userinfo{}, false, err
		}

		//md5解密
		m5 := md5.New()
		m5.Write([]byte(password))
		m5.Write([]byte(userinfo.Salt))
		st := m5.Sum(nil)
		hashPwd := hex.EncodeToString(st)

		if hashPwd != userinfo.Password {
			return model.Userinfo{}, false, nil
		}
		return userinfo, true, nil
	} else {
		//手机号登录

		userinfo, err := d.QueryByPhone(loginName)
		if err != nil {
			if err.Error() == "sql: no rows in result set" {
				return model.Userinfo{}, false, nil
			}
			return model.Userinfo{}, false, err
		}

		//md5解密
		m5 := md5.New()
		m5.Write([]byte(password))
		m5.Write([]byte(userinfo.Salt))
		st := m5.Sum(nil)
		hashPwd := hex.EncodeToString(st)

		if hashPwd != userinfo.Password {
			return model.Userinfo{}, false, nil
		}

		return userinfo, true, nil
	}
}

//检验用户名是否存在, false不存在 反之存在
func (u *UserService) JudgeUsername(username string) (bool, error) {
	d := dao.UserDao{tool.GetDb()}
	_, err := d.QueryByUsername(username)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

//检验手机是否存在, false不存在 反之存在
func (u *UserService) JudgePhone(phone string) (bool, error) {
	d := dao.UserDao{tool.GetDb()}
	_, err := d.QueryByPhone(phone)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

//检验邮箱是否存在, false不存在 反之存在
func (u *UserService) JudgeEmail(email string) (bool, error) {
	d := dao.UserDao{tool.GetDb()}
	email = strings.ToLower(email)
	_, err := d.QueryByEmail(email)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

//检验验证码是否正确
func (u *UserService) JudgeVerifyCode(ctx *gin.Context, key string, givenValue string) (bool, error) {
	rd := dao.RedisDao{}
	value, err := rd.RedisGetValue(ctx, key)
	if err != nil {
		return false, err
	}

	if value != givenValue {
		return false, nil
	}

	return true, nil

}

//验证码放入redis中
func (u *UserService) VerifyCodeIn(ctx *gin.Context, key string, value string) error {
	rd := dao.RedisDao{}
	err := rd.RedisSetValue(ctx, key, value)
	return err
}

//注册实体放入mysql
func (u *UserService) RegisterModelIn(userinfo model.Userinfo) error {
	d := dao.UserDao{tool.GetDb()}
	err := d.InsertUser(userinfo)
	return err
}

//通过手机号发送验证码
func (u *UserService) SendCodeByPhone(phone string) (string, error) {
	code := fmt.Sprintf("%06v", rand.New(rand.NewSource(time.Now().UnixNano())).Int31n(1000000))

	//调用阿里云sdk
	cfg := tool.GetCfg().Sms
	//fmt.Println("asdfsadf", cfg.AppSecret, cfg.AppKey)
	client, err := dysmsapi.NewClientWithAccessKey(cfg.RegionId, cfg.AppKey, cfg.AppSecret)
	if err != nil {
		return "", err
	}

	request := dysmsapi.CreateSendSmsRequest()
	request.Scheme = "https"
	request.SignName = cfg.SignName
	request.TemplateCode = cfg.TemplateCode
	request.PhoneNumbers = phone

	par, err := json.Marshal(gin.H{
		"code": code,
	})

	request.TemplateParam = string(par)

	response, err := client.SendSms(request)
	fmt.Println(response)

	if err != nil {
		return "", err
	}

	//成功
	if response.Code == "OK" {
		return code, nil
	}

	if response.Code == "isv.MOBILE_NUMBER_ILLEGAL" {
		return "isv.MOBILE_NUMBER_ILLEGAL", nil
	}

	return "", nil
}
