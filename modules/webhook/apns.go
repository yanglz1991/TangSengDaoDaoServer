package webhook

import (
	"errors"

	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/certificate"
	"github.com/sideshow/apns2/token"
)

// APNS APNS
type APNS struct {
	client      *apns2.Client
	p12FilePath string
	p8FilePath  string
	keyID       string
	teamID      string
	password    string
	dev         bool // 是否是开发环境

}

// NewAPNS NewAPNS
func NewAPNS(p12FilePath, password string, dev bool) *APNS {
	apns := &APNS{
		p12FilePath: p12FilePath,
		password:    password,
		dev:         dev,
	}
	return apns
}

// NewAPNSWithP8 NewAPNSWithP8
func NewAPNSWithP8(p8FilePath, keyID, teamID string, dev bool) *APNS {
	apns := &APNS{
		p8FilePath: p8FilePath,
		keyID:      keyID,
		teamID:     teamID,
		dev:        dev,
	}
	return apns
}

func (a *APNS) createClient() (*apns2.Client, error) {
	if a.p8FilePath != "" {
		authKey, err := token.AuthKeyFromFile(a.p8FilePath)
		if err != nil {
			return nil, err
		}
		tok := &token.Token{AuthKey: authKey, KeyID: a.keyID, TeamID: a.teamID}
		var client *apns2.Client
		if a.dev {
			client = apns2.NewTokenClient(tok).Development()
		} else {
			client = apns2.NewTokenClient(tok).Production()
		}
		return client, nil
	}
	cert, err := certificate.FromP12File(a.p12FilePath, a.password)
	if err != nil {
		return nil, err
	}
	var client *apns2.Client
	if a.dev {
		client = apns2.NewClient(cert).Development()
	} else {
		client = apns2.NewClient(cert).Production()
	}
	return client, nil
}

// Push 推送消息
func (a *APNS) Push(notification *apns2.Notification) error {
	var err error
	if a.client == nil {
		a.client, err = a.createClient()
		if err != nil {
			return err
		}
	}
	res, err := a.client.Push(notification)
	if err != nil {
		return err
	}
	if res.StatusCode != 200 {
		return errors.New(res.Reason)
	}

	return nil

}
