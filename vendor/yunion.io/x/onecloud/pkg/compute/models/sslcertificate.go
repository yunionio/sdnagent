// Copyright 2019 Yunion
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package models

import (
	"context"
	"time"

	"yunion.io/x/cloudmux/pkg/cloudprovider"
	"yunion.io/x/jsonutils"
	"yunion.io/x/pkg/errors"
	"yunion.io/x/pkg/util/compare"
	"yunion.io/x/sqlchemy"

	"yunion.io/x/onecloud/pkg/apis"
	api "yunion.io/x/onecloud/pkg/apis/compute"
	"yunion.io/x/onecloud/pkg/cloudcommon/db"
	"yunion.io/x/onecloud/pkg/cloudcommon/db/lockman"
	"yunion.io/x/onecloud/pkg/cloudcommon/db/taskman"
	"yunion.io/x/onecloud/pkg/cloudcommon/notifyclient"
	"yunion.io/x/onecloud/pkg/cloudcommon/validators"
	"yunion.io/x/onecloud/pkg/httperrors"
	"yunion.io/x/onecloud/pkg/mcclient"
	"yunion.io/x/onecloud/pkg/util/stringutils2"
)

// +onecloud:swagger-gen-model-singular=sslcertificate
// +onecloud:swagger-gen-model-plural=sslcertificates
type SSSLCertificateManager struct {
	db.SVirtualResourceBaseManager
	db.SExternalizedResourceBaseManager
	SDeletePreventableResourceBaseManager

	SManagedResourceBaseManager
	SDnsZoneResourceBaseManager
}

var SSLCertificateManager *SSSLCertificateManager

func init() {
	SSLCertificateManager = &SSSLCertificateManager{
		SVirtualResourceBaseManager: db.NewVirtualResourceBaseManager(
			SSSLCertificate{},
			"sslcertificates_tbl",
			"sslcertificate",
			"sslcertificates",
		),
	}
	SSLCertificateManager.SetVirtualObject(SSLCertificateManager)
}

type SSSLCertificate struct {
	db.SVirtualResourceBase
	db.SExternalizedResourceBase
	SManagedResourceBase
	SDnsZoneResourceBase

	SDeletePreventableResourceBase

	Sans        string    `width:"2048" charset:"utf8" nullable:"false" list:"user" create:"required"`
	StartDate   time.Time `list:"user"`
	Province    string    `width:"64" charset:"utf8" nullable:"false" list:"user" create:"optional"`
	Common      string    `width:"128" charset:"utf8" nullable:"false" list:"user" create:"optional"`
	Country     string    `width:"32" charset:"utf8" nullable:"false" list:"user" create:"optional"`
	Issuer      string    `width:"128" charset:"utf8" nullable:"false" list:"user" create:"required"`
	IsUpload    bool      `list:"user" create:"optional"`
	EndDate     time.Time `list:"user" create:"optional"`
	Fingerprint string    `width:"128" charset:"utf8" nullable:"false" list:"user" create:"optional"`
	City        string    `width:"64" charset:"utf8" nullable:"false" list:"user" create:"optional"`
	OrgName     string    `width:"256" charset:"utf8" nullable:"false" list:"user" create:"optional"`
	Certificate string    `charset:"utf8" nullable:"true" list:"user" create:"optional"`
	PrivateKey  string    `charset:"utf8" nullable:"true" list:"user" create:"optional"`
}

func (s SSSLCertificate) GetExternalId() string {
	return s.ExternalId
}

func (man *SSSLCertificateManager) FetchCustomizeColumns(
	ctx context.Context,
	userCred mcclient.TokenCredential,
	query jsonutils.JSONObject,
	objs []interface{},
	fields stringutils2.SSortedStrings,
	isList bool,
) []api.SSLCertificateDetails {
	rows := make([]api.SSLCertificateDetails, len(objs))
	virtRows := man.SVirtualResourceBaseManager.FetchCustomizeColumns(ctx, userCred, query, objs, fields, isList)
	manRows := man.SManagedResourceBaseManager.FetchCustomizeColumns(ctx, userCred, query, objs, fields, isList)

	for i := range rows {
		rows[i] = api.SSLCertificateDetails{
			VirtualResourceDetails: virtRows[i],
			ManagedResourceInfo:    manRows[i],
		}
		cert := objs[i].(*SSSLCertificate)
		rows[i].IsExpired = cert.EndDate.Before(time.Now())
	}

	return rows
}

func (man *SSSLCertificateManager) GetContextManagers() [][]db.IModelManager {
	return [][]db.IModelManager{
		{CloudproviderManager},
	}
}

// SSLCertificate实例列表
func (man *SSSLCertificateManager) ListItemFilter(
	ctx context.Context,
	q *sqlchemy.SQuery,
	userCred mcclient.TokenCredential,
	query api.SSLCertificateListInput,
) (*sqlchemy.SQuery, error) {
	var err error
	q, err = man.SVirtualResourceBaseManager.ListItemFilter(ctx, q, userCred, query.VirtualResourceListInput)
	if err != nil {
		return nil, errors.Wrap(err, "SVirtualResourceBaseManager.ListItemFilter")
	}

	q, err = man.SExternalizedResourceBaseManager.ListItemFilter(ctx, q, userCred, query.ExternalizedResourceBaseListInput)
	if err != nil {
		return nil, errors.Wrap(err, "SExternalizedResourceBaseManager.ListItemFilter")
	}

	q, err = man.SDeletePreventableResourceBaseManager.ListItemFilter(ctx, q, userCred, query.DeletePreventableResourceBaseListInput)
	if err != nil {
		return nil, errors.Wrap(err, "SDeletePreventableResourceBaseManager.ListItemFilter")
	}

	q, err = man.SManagedResourceBaseManager.ListItemFilter(ctx, q, userCred, query.ManagedResourceListInput)
	if err != nil {
		return nil, errors.Wrap(err, "SManagedResourceBaseManager.ListItemFilter")
	}

	q, err = man.SDnsZoneResourceBaseManager.ListItemFilter(ctx, q, userCred, query.DnsZoneFilterListBase)
	if err != nil {
		return nil, errors.Wrap(err, "SDnsZoneResourceBaseManager.ListItemFilter")
	}

	if query.IsExpired != nil {
		if *query.IsExpired {
			q = q.LT("end_date", time.Now())
		} else {
			q = q.GE("end_date", time.Now())
		}
	}

	return q, nil
}

func (man *SSSLCertificateManager) OrderByExtraFields(
	ctx context.Context,
	q *sqlchemy.SQuery,
	userCred mcclient.TokenCredential,
	query api.SSLCertificateListInput,
) (*sqlchemy.SQuery, error) {
	q, err := man.SVirtualResourceBaseManager.OrderByExtraFields(ctx, q, userCred, query.VirtualResourceListInput)
	if err != nil {
		return nil, errors.Wrap(err, "SVirtualResourceBaseManager.OrderByExtraFields")
	}

	q, err = man.SManagedResourceBaseManager.OrderByExtraFields(ctx, q, userCred, query.ManagedResourceListInput)
	if err != nil {
		return nil, errors.Wrap(err, "SManagedResourceBaseManager.OrderByExtraFields")
	}

	return q, nil
}

func (man *SSSLCertificateManager) QueryDistinctExtraField(q *sqlchemy.SQuery, field string) (*sqlchemy.SQuery, error) {
	q, err := man.SVirtualResourceBaseManager.QueryDistinctExtraField(q, field)
	if err == nil {
		return q, nil
	}

	q, err = man.SManagedResourceBaseManager.QueryDistinctExtraField(q, field)
	if err == nil {
		return q, nil
	}

	q, err = man.SDnsZoneResourceBaseManager.QueryDistinctExtraField(q, field)
	if err == nil {
		return q, nil
	}

	return q, httperrors.ErrNotFound
}

func (man *SSSLCertificateManager) ValidateCreateData(
	ctx context.Context,
	userCred mcclient.TokenCredential,
	ownerId mcclient.IIdentityProvider,
	query jsonutils.JSONObject,
	input *api.SSLCertificateCreateInput,
) (*api.SSLCertificateCreateInput, error) {
	if len(input.DnsZoneId) > 0 {
		obj, err := validators.ValidateModel(ctx, userCred, DnsZoneManager, &input.DnsZoneId)
		if err != nil {
			return nil, err
		}
		input.DnsZoneId = obj.GetId()
	}
	var err error
	input.VirtualResourceCreateInput, err = man.SVirtualResourceBaseManager.ValidateCreateData(ctx, userCred, ownerId, query, input.VirtualResourceCreateInput)
	if err != nil {
		return nil, err
	}

	return input, nil
}

func (self *SSSLCertificate) PostCreate(ctx context.Context, userCred mcclient.TokenCredential, ownerId mcclient.IIdentityProvider, query jsonutils.JSONObject, data jsonutils.JSONObject) {
	self.SVirtualResourceBase.PostCreate(ctx, userCred, ownerId, query, data)
	self.StartSSLCertificateCreateTask(ctx, userCred, "")
}

func (self *SSSLCertificate) StartSSLCertificateCreateTask(ctx context.Context, userCred mcclient.TokenCredential, parentTaskId string) error {
	params := jsonutils.NewDict()
	task, err := taskman.TaskManager.NewTask(ctx, "SSLCertificateCreateTask", self, userCred, params, parentTaskId, "", nil)
	if err != nil {
		return errors.Wrap(err, "NewTask")
	}
	self.SetStatus(ctx, userCred, apis.STATUS_CREATING, "")
	return task.ScheduleRun(nil)
}

func (self *SSSLCertificate) GetDnsZone() (*SDnsZone, error) {
	if len(self.DnsZoneId) == 0 {
		return nil, errors.Wrapf(cloudprovider.ErrNotFound, "DnsZoneId is empty")
	}
	zone, err := DnsZoneManager.FetchById(self.DnsZoneId)
	if err != nil {
		return nil, errors.Wrapf(err, "DnsZoneManager.FetchById")
	}
	return zone.(*SDnsZone), nil
}

func (r *SCloudprovider) GetSSLCertificates() ([]SSSLCertificate, error) {
	q := SSLCertificateManager.Query().Equals("manager_id", r.Id)
	ret := make([]SSSLCertificate, 0)
	err := db.FetchModelObjects(SSLCertificateManager, q, &ret)
	if err != nil {
		return nil, errors.Wrapf(err, "db.FetchModelObjects")
	}
	return ret, nil
}

func (r *SCloudprovider) SyncSSLCertificates(
	ctx context.Context,
	userCred mcclient.TokenCredential,
	exts []cloudprovider.ICloudSSLCertificate,
) compare.SyncResult {
	// 加锁防止重入
	lockman.LockRawObject(ctx, SSLCertificateManager.KeywordPlural(), r.Id)
	defer lockman.ReleaseRawObject(ctx, SSLCertificateManager.KeywordPlural(), r.Id)

	result := compare.SyncResult{}

	dbEss, err := r.GetSSLCertificates()
	if err != nil {
		result.Error(err)
		return result
	}

	removed := make([]SSSLCertificate, 0)
	commondb := make([]SSSLCertificate, 0)
	commonext := make([]cloudprovider.ICloudSSLCertificate, 0)
	added := make([]cloudprovider.ICloudSSLCertificate, 0)
	// 本地和云上资源列表进行比对
	err = compare.CompareSets(dbEss, exts, &removed, &commondb, &commonext, &added)
	if err != nil {
		result.Error(err)
		return result
	}

	// 删除云上没有的资源
	for i := 0; i < len(removed); i++ {
		err := removed[i].syncRemoveCloudSSLCertificate(ctx, userCred)
		if err != nil {
			result.DeleteError(err)
			continue
		}
		result.Delete()
	}

	// 和云上资源属性进行同步
	for i := 0; i < len(commondb); i++ {
		err := commondb[i].SyncWithCloudSSLCertificate(ctx, userCred, commonext[i])
		if err != nil {
			result.UpdateError(err)
			continue
		}
		result.Update()
	}

	// 创建本地没有的云上资源
	for i := 0; i < len(added); i++ {
		_, err := r.newFromCloudSSLCertificate(ctx, userCred, added[i])
		if err != nil {
			result.AddError(err)
			continue
		}
		result.Add()
	}
	return result
}

func (s *SSSLCertificate) syncRemoveCloudSSLCertificate(ctx context.Context, userCred mcclient.TokenCredential) error {
	err := s.RealDelete(ctx, userCred)
	if err != nil {
		return err
	}
	notifyclient.EventNotify(ctx, userCred, notifyclient.SEventNotifyParam{
		Obj:    s,
		Action: notifyclient.ActionSyncDelete,
	})
	return nil
}

func (s *SSSLCertificate) RealDelete(ctx context.Context, userCred mcclient.TokenCredential) error {
	return s.SVirtualResourceBase.Delete(ctx, userCred)
}

// 同步资源属性
func (s *SSSLCertificate) SyncWithCloudSSLCertificate(ctx context.Context, userCred mcclient.TokenCredential, ext cloudprovider.ICloudSSLCertificate) error {
	diff, err := db.UpdateWithLock(ctx, s, func() error {
		s.ExternalId = ext.GetGlobalId()
		s.Status = ext.GetStatus()
		s.Name = ext.GetName()
		s.Sans = ext.GetSans()
		s.StartDate = ext.GetStartDate()
		s.Province = ext.GetProvince()
		s.Common = ext.GetCommon()
		s.Country = ext.GetCountry()
		s.Issuer = ext.GetIssuer()
		s.IsUpload = ext.GetIsUpload()
		s.EndDate = ext.GetEndDate()
		s.Fingerprint = ext.GetFingerprint()
		s.City = ext.GetCity()
		s.OrgName = ext.GetOrgName()
		s.Certificate = ext.GetCert()
		s.PrivateKey = ext.GetKey()
		if zoneId := ext.GetDnsZoneId(); len(zoneId) > 0 {
			zone, err := db.FetchByExternalIdAndManagerId(DnsZoneManager, zoneId, func(q *sqlchemy.SQuery) *sqlchemy.SQuery {
				return q.Equals("manager_id", s.ManagerId)
			})
			if err == nil {
				s.DnsZoneId = zone.GetId()
			}
		}
		return nil
	})
	if err != nil {
		return errors.Wrapf(err, "db.Update")
	}

	if len(diff) > 0 {
		notifyclient.EventNotify(ctx, userCred, notifyclient.SEventNotifyParam{
			Obj:    s,
			Action: notifyclient.ActionSyncUpdate,
		})
	}

	if account := s.GetCloudaccount(); account != nil {
		syncVirtualResourceMetadata(ctx, userCred, s, ext, account.ReadOnly)
	}

	if provider := s.GetCloudprovider(); provider != nil {
		SyncCloudProject(ctx, userCred, s, provider.GetOwnerId(), ext, provider)
	}
	db.OpsLog.LogSyncUpdate(s, diff, userCred)
	return nil
}

func (r *SCloudprovider) newFromCloudSSLCertificate(
	ctx context.Context,
	userCred mcclient.TokenCredential,
	ext cloudprovider.ICloudSSLCertificate,
) (*SSSLCertificate, error) {
	s := SSSLCertificate{}
	s.SetModelManager(SSLCertificateManager, &s)

	s.ExternalId = ext.GetGlobalId()
	s.ManagerId = r.Id
	s.IsEmulated = ext.IsEmulated()
	s.Status = ext.GetStatus()
	s.Name = ext.GetName()
	s.Sans = ext.GetSans()
	s.StartDate = ext.GetStartDate()
	s.Province = ext.GetProvince()
	s.Common = ext.GetCommon()
	s.Country = ext.GetCountry()
	s.Issuer = ext.GetIssuer()
	//s.Expired = ext.GetExpired()
	s.IsUpload = ext.GetIsUpload()
	s.EndDate = ext.GetEndDate()
	s.Fingerprint = ext.GetFingerprint()
	s.City = ext.GetCity()
	s.OrgName = ext.GetOrgName()
	s.Certificate = ext.GetCert()
	s.PrivateKey = ext.GetKey()
	if zoneId := ext.GetDnsZoneId(); len(zoneId) > 0 {
		zone, err := db.FetchByExternalIdAndManagerId(DnsZoneManager, zoneId, func(q *sqlchemy.SQuery) *sqlchemy.SQuery {
			return q.Equals("manager_id", r.Id)
		})
		if err == nil {
			s.DnsZoneId = zone.GetId()
		}
	}

	if createdAt := ext.GetCreatedAt(); !createdAt.IsZero() {
		s.CreatedAt = createdAt
	}

	var err error
	err = func() error {
		// 这里加锁是为了防止名称重复
		lockman.LockRawObject(ctx, SSLCertificateManager.Keyword(), "name")
		defer lockman.ReleaseRawObject(ctx, SSLCertificateManager.Keyword(), "name")

		s.Name, err = db.GenerateName(ctx, SSLCertificateManager, r.GetOwnerId(), ext.GetName())
		if err != nil {
			return errors.Wrapf(err, "db.GenerateName")
		}

		return SSLCertificateManager.TableSpec().Insert(ctx, &s)
	}()
	if err != nil {
		return nil, errors.Wrapf(err, "newFromCloudSSLCertificate.Insert")
	}

	notifyclient.EventNotify(ctx, userCred, notifyclient.SEventNotifyParam{
		Obj:    &s,
		Action: notifyclient.ActionSyncCreate,
	})
	// 同步标签
	_ = syncVirtualResourceMetadata(ctx, userCred, &s, ext, false)
	// 同步项目归属
	SyncCloudProject(ctx, userCred, &s, r.GetOwnerId(), ext, r)

	db.OpsLog.LogEvent(&s, db.ACT_CREATE, s.GetShortDesc(ctx), userCred)

	return &s, nil
}

func (man *SSSLCertificateManager) ListItemExportKeys(ctx context.Context,
	q *sqlchemy.SQuery,
	userCred mcclient.TokenCredential,
	keys stringutils2.SSortedStrings,
) (*sqlchemy.SQuery, error) {
	var err error

	q, err = man.SVirtualResourceBaseManager.ListItemExportKeys(ctx, q, userCred, keys)
	if err != nil {
		return nil, errors.Wrap(err, "SVirtualResourceBaseManager.ListItemExportKeys")
	}

	if keys.ContainsAny(man.SManagedResourceBaseManager.GetExportKeys()...) {
		q, err = man.SManagedResourceBaseManager.ListItemExportKeys(ctx, q, userCred, keys)
		if err != nil {
			return nil, errors.Wrap(err, "SManagedResourceBaseManager.ListItemExportKeys")
		}
	}

	return q, nil
}
