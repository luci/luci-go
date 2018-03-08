package bq

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"cloud.google.com/go/bigquery"
	"go.chromium.org/gae/service/memcache"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"golang.org/x/net/context"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

func RunQuery(c context.Context, query string, cacheTime time.Duration, result interface{}) error {
	if !reflect.ValueOf(result).Elem().CanSet() {
		return fmt.Errorf("result must be settable (a pointer)")
	}
	resultType := reflect.TypeOf(result).Elem()
	if resultType.Kind() != reflect.Slice {
		return fmt.Errorf("result must be a slice, is %v", resultType)
	}
	elemT := resultType.Elem()

	h := sha256.New()
	h.Write([]byte(query))
	itm := memcache.NewItem(c, fmt.Sprintf("query|%x", h.Sum(nil)))
	if cacheTime > 0 {
		itm.SetExpiration(cacheTime)
	}
	logging.Debugf(c, "caching query for %d minutes", cacheTime/time.Minute)
	if err := memcache.Get(c, itm); err != nil {
		if err != memcache.ErrCacheMiss {
			logging.Warningf(c, "got error while reading query result from memcache: %v", err)
		}
		c, cancel := context.WithTimeout(c, 10*time.Minute)
		defer cancel()
		tr, err := auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(bigquery.Scope))
		if err != nil {
			panic(err)
		}
		cl, err := bigquery.NewClient(c, "chrome-infra-events", option.WithHTTPClient(&http.Client{
			Transport: tr,
			Timeout:   10 * time.Minute,
		}))
		if err != nil {
			panic(err)
		}
		q := cl.Query(query)
		q.UseLegacySQL = false
		job, err := q.Run(c)
		if err != nil {
			panic(err)
		}
		logging.Infof(c, "running job %v", job.ID())

		// finalRes = &bigquery.GetQueryResultsResponse{
		// 	JobComplete: res.JobComplete,
		// 	Rows:        res.Rows,
		// 	Schema:      res.Schema,
		// }
		it, err := job.Read(c)
		if err != nil {
			panic(err)
		}

		firstItm := reflect.New(elemT.Elem())
		err = it.Next(firstItm.Interface())
		if err == iterator.Done {
			panic(fmt.Sprintf("itm is %v", firstItm.Interface()))
		}
		resultSlice := reflect.MakeSlice(resultType, int(it.TotalRows), int(it.TotalRows))
		resultSlice.Index(0).Set(firstItm)

		logging.Infof(c, "getting results for %v (%v total)", job.ID(), it.TotalRows)
		for i := 1; ; i++ {
			madeItm := reflect.New(elemT.Elem())
			err = it.Next(madeItm.Interface())
			if err == iterator.Done {
				break
			}
			if err != nil {
				panic(err)
			}
			resultSlice.Index(i).Set(madeItm)
		}
		reflect.ValueOf(result).Elem().Set(resultSlice)
		jsonData, err := json.Marshal(result)
		if err != nil {
			logging.Warningf(c, "while trying to json.Marshal query results: %v", err)
		}
		itm.SetValue(jsonData)
		if err := memcache.Set(c, itm); err != nil {
			return fmt.Errorf("error while writing result to memcache: %v", err)
		}
	}
	if reflect.ValueOf(result).Elem().Len() == 0 {
		if len(itm.Value()) == 0 {
			logging.Warningf(c, "query completed but no results")
			return nil
		}
		if err := json.Unmarshal(itm.Value(), result); err != nil {
			return fmt.Errorf("error while reading result from memcache: %v", err)
		}
	}
	return nil
}
