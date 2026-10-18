package repository

import (
 "context"
 "testing"
 "time"

 "github.com/DATA-DOG/go-sqlmock"
 "github.com/stretchr/testify/require"
)

func TestMonitorHealthCombinesRealAndActiveFactsWithSampleCounts(t *testing.T) {
 db,mock,err:=sqlmock.New();require.NoError(t,err);defer db.Close()
 repo:=&channelMonitorRepository{db:db}
 mock.ExpectQuery(`WITH targets AS`).WithArgs(sqlmock.AnyArg(),7).WillReturnRows(sqlmock.NewRows([]string{"monitor_id","model","total","ok","avg"}).AddRow(3,"new-model",3,2,1500))
 now:=time.Now().UTC()
 mock.ExpectQuery(`WITH targets AS`).WithArgs(sqlmock.AnyArg(),30,60).WillReturnRows(sqlmock.NewRows([]string{"monitor_id","model","status","latency","first","output_rate","at","source"}).AddRow(3,"new-model","error",2000,nil,nil,now,"user_request").AddRow(3,"new-model","operational",1000,800,2,now.Add(-time.Second),"probe"))
 got,err:=repo.ReadMonitorHealth(context.Background(),[]int64{3,4},7,60)
 require.NoError(t,err)
 require.Equal(t,3,got[3].Availability.TotalChecks)
 require.InDelta(t,200.0/3,got[3].Availability.AvailabilityPct,1e-9)
 require.Len(t,got[3].Recent,2)
 require.Equal(t,"user_request",got[3].Recent[0].Source)
 require.Nil(t,got[3].Recent[0].FirstTokenMs)
 require.Equal(t,800,*got[3].Recent[1].FirstTokenMs)
 require.Equal(t,2.0,*got[3].Recent[1].OutputTokensPerSecond)
 require.Nil(t,got[3].Recent[0].OutputTokensPerSecond)
 require.Zero(t,got[4].Availability.TotalChecks)
 require.NoError(t,mock.ExpectationsWereMet())
}

func TestMonitorHealthKeepsErrorsWithoutResolvedModel(t *testing.T) {
 db,mock,err:=sqlmock.New();require.NoError(t,err);defer db.Close()
 repo:=&channelMonitorRepository{db:db}
 // Both reads must normalize nullable model names without dropping failures.
 query:=`(?s)WITH targets AS.*COALESCE\(NULLIF\(u.requested_model,''\),u.model,''\).*COALESCE\(NULLIF\(e.requested_model,''\),e.model,''\)`
 mock.ExpectQuery(query).WithArgs(sqlmock.AnyArg(),7).WillReturnRows(sqlmock.NewRows([]string{"monitor_id","model","total","ok","avg"}).AddRow(3,"",1,0,nil))
 now:=time.Now().UTC()
 mock.ExpectQuery(query).WithArgs(sqlmock.AnyArg(),30,60).WillReturnRows(sqlmock.NewRows([]string{"monitor_id","model","status","latency","first","output_rate","at","source"}).AddRow(3,"","error",nil,nil,nil,now,"user_request"))
 got,err:=repo.ReadMonitorHealth(context.Background(),[]int64{3},7,60)
 require.NoError(t,err)
 require.Equal(t,1,got[3].Availability.TotalChecks)
 require.Zero(t,got[3].Availability.OperationalChecks)
 require.Zero(t,got[3].Availability.AvailabilityPct)
 require.Equal(t,1,got[3].Models[""].TotalChecks)
 require.Len(t,got[3].Recent,1)
 require.Empty(t,got[3].Recent[0].Model)
 require.Equal(t,"error",got[3].Recent[0].Status)
 require.Equal(t,"user_request",got[3].Recent[0].Source)
 require.NoError(t,mock.ExpectationsWereMet())
}

func TestMonitorGroupCacheUsesOneSnapshotForBucketsAndSampleCount(t *testing.T) {
 db,mock,err:=sqlmock.New();require.NoError(t,err);defer db.Close()
 repo:=&channelMonitorRepository{db:db}
 mock.ExpectQuery(`COUNT\(ul.id\) FILTER`).WithArgs(int64(9),int64(86400)).WillReturnRows(sqlmock.NewRows([]string{"rate","type","peak","mult","start","end","read","write","input","samples","at"}).AddRow(0.07,"standard",false,1.0,"","",300,100,600,2,time.Now()))
 got,err:=repo.GetGroupMetrics(context.Background(),9,24*time.Hour)
 require.NoError(t,err);require.Equal(t,int64(2),got.CacheSampleCount);require.Equal(t,int64(86400),got.CacheWindowSeconds);require.Equal(t,30.0,got.CacheHitRate)
 require.NoError(t,mock.ExpectationsWereMet())
}

func TestMonitorHealthQueryExcludesProbesAndFailedStreamDuplicates(t *testing.T) {
 require.Contains(t,monitorHealthFactsSQL,"p.provisioned_api_key_id=u.api_key_id")
 require.Contains(t,monitorHealthFactsSQL,"NOT e.resolved")
 require.Contains(t,monitorHealthFactsSQL,"NOT e.is_business_limited")
 require.Contains(t,monitorHealthFactsSQL,"SELECT DISTINCT ON (e.api_key_id")
 require.Contains(t,monitorHealthFactsSQL,"'local:'||e.request_id")
 require.Contains(t,monitorHealthFactsSQL,"e.request_id=u.request_id")
 require.Contains(t,monitorHealthFactsSQL,"FROM targets t JOIN channel_monitor_histories")
}
