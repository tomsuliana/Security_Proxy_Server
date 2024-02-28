package repository

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type RequestSaver interface {
	Save(*http.Request) (string, error)
	Get(id string) (*Request, error)
	GetEncoded(id string) (*http.Request, error)
	List(limit int64) ([]*Request, error)
}

type ResponseSaver interface {
	Save(requestId string, resp *http.Response) (string, error)
	Get(id string) (*Response, error)
	GetByRequest(requestId string) (*Response, error)
	List(limit int64) ([]*Response, error)
}

const kDatabase = "http-proxy"

type Request struct {
	Id         primitive.ObjectID `json:"id" bson:"_id"`
	Scheme     string             `json:"scheme"`
	Host       string             `json:"host"`
	Method     string             `json:"method"`
	Path       string             `json:"path"`
	Cookies    map[string]string  `json:"cookies"`
	Body       string             `json:"body,omitempty" bson:"body,omitempty"`
	Headers    bson.M             `json:"headers"`
	GetParams  bson.M             `json:"get_params" bson:"get_params"`
	PostParams bson.M             `json:"post_params" bson:"post_params"`
}

type Response struct {
	Id        primitive.ObjectID `json:"id" bson:"_id"`
	RequestId primitive.ObjectID `json:"request_id" bson:"request_id"`
	Code      int                `json:"code"`
	Message   string             `json:"message"`
	Body      string             `json:"body,omitempty" bson:"body,omitempty"`
	Headers   bson.M             `json:"headers"`
}

const kRequests = "requests"
const kResponses = "responses"

type MongoRequestSaver struct {
	requests *mongo.Collection
}

type MongoResponseSaver struct {
	responses *mongo.Collection
}

func NewMongoRequestSaver(conn *mongo.Client) RequestSaver {
	return &MongoRequestSaver{
		requests: conn.Database(kDatabase).Collection(kRequests),
	}
}

func NewMongoResponseSaver(conn *mongo.Client) ResponseSaver {
	return &MongoResponseSaver{
		responses: conn.Database(kDatabase).Collection(kResponses),
	}
}

func (s *MongoRequestSaver) Save(req *http.Request) (string, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return "", err
	}

	req.Body = io.NopCloser(bytes.NewReader(body))

	req.URL.RawQuery = strings.ReplaceAll(req.URL.RawQuery, ";", "&")
	rawQuery := toBson(req.URL.Query())

	headers := toBson(req.Header)
	delete(headers, "Cookie")

	cookieMap := make(map[string]string, 4)

	for _, cookie := range req.Cookies() {
		cookieMap[cookie.Name] = cookie.Value
	}

	value := bson.M{
		"method":     req.Method,
		"scheme":     req.URL.Scheme,
		"host":       req.Host,
		"path":       req.URL.Path,
		"get_params": rawQuery,
		"headers":    headers,
		"cookies":    cookieMap,
	}

	postParams, err := parsePostParams(req)
	if err != nil {
		return "", err
	}
	if len(postParams) != 0 {
		value["post_params"] = postParams
		req.Body = io.NopCloser(bytes.NewReader(body))
	} else {
		if req.Body != nil {
			value["body"] = string(body)
		}
	}

	res, err := s.requests.InsertOne(context.Background(), value)
	if err != nil {
		return "", err
	}

	return res.InsertedID.(primitive.ObjectID).Hex(), nil
}

func (s *MongoRequestSaver) GetEncoded(id string) (*http.Request, error) {
	value, err := s.Get(id)
	if err != nil {
		return nil, err
	}

	return toRequest(value)
}

func (s *MongoRequestSaver) Get(id string) (*Request, error) {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	res := s.requests.FindOne(context.Background(), bson.D{{Key: "_id", Value: objectId}})
	value := &Request{}

	err = res.Decode(value)
	if err != nil {
		return nil, err
	}

	return value, nil
}

func (s *MongoRequestSaver) List(limit int64) ([]*Request, error) {
	ctx := context.Background()

	opts := options.Find().SetLimit(limit).SetSort(bson.D{{Key: "_id", Value: -1}})
	cursor, err := s.requests.Find(ctx, bson.D{}, opts)
	if err != nil {
		return nil, err
	}

	res := make([]*Request, 0, limit/2)

	err = cursor.All(ctx, &res)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func parsePostParams(req *http.Request) (bson.M, error) {
	if req.Body == nil {
		return nil, nil
	}

	err := req.ParseForm()
	if err != nil {
		return nil, err
	}

	return toBson(req.PostForm), nil
}

func toBson(values map[string][]string) bson.M {
	res := make(bson.M, len(values))

	for key, value := range values {
		if len(value) == 1 {
			res[key] = value[0]
		} else {
			res[key] = value
		}
	}
	return res
}

func toRequest(data *Request) (*http.Request, error) {
	res, err := http.NewRequest(data.Method, data.Scheme+"://"+data.Host+data.Path, nil)
	if err != nil {
		return nil, err
	}

	res.Host = data.Host

	res.Header = fromBson(data.Headers)
	for key, value := range data.Cookies {
		res.AddCookie(&http.Cookie{Name: key, Value: value})
	}
	res.URL.RawQuery = url.Values(fromBson(data.GetParams)).Encode()
	res.Body = getBody(res, data)

	return res, nil
}

func getBody(req *http.Request, data *Request) io.ReadCloser {
	if req.Header.Get("Content-Type") == "application/x-www-form-urlencoded" {
		var params url.Values = fromBson(data.PostParams)
		return io.NopCloser(strings.NewReader(params.Encode()))
	}

	return io.NopCloser(strings.NewReader(data.Body))
}

func fromBson(values bson.M) map[string][]string {
	res := make(map[string][]string, len(values))

	for key, value := range values {
		str, ok := value.(string)
		if ok {
			res[key] = []string{str}
		}

		arr, ok := value.(bson.A)
		if !ok {
			continue
		}

		res[key] = make([]string, len(arr))
		for _, elem := range arr {
			str, ok := elem.(string)
			if !ok {
				continue
			}
			res[key] = append(res[key], str)
		}
	}

	return res
}

func (s *MongoResponseSaver) Save(requestId string, resp *http.Response) (string, error) {
	requestObjectId, err := primitive.ObjectIDFromHex(requestId)
	if err != nil {
		return "", err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	resp.Body = io.NopCloser(bytes.NewReader(body))

	res, err := s.responses.InsertOne(context.Background(), bson.M{
		"code":       resp.StatusCode,
		"message":    resp.Status[strings.Index(resp.Status, " ")+1:],
		"headers":    toBson(resp.Header),
		"request_id": requestObjectId,
		"body":       string(body),
	})
	if err != nil {
		return "", err
	}

	return res.InsertedID.(primitive.ObjectID).Hex(), nil
}

func (s *MongoResponseSaver) Get(id string) (*Response, error) {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	res := s.responses.FindOne(context.Background(), bson.D{{Key: "_id", Value: objectId}})
	value := &Response{}

	err = res.Decode(value)
	if err != nil {
		return nil, err
	}

	return value, nil
}

func (s *MongoResponseSaver) GetByRequest(requestId string) (*Response, error) {
	objectId, err := primitive.ObjectIDFromHex(requestId)
	if err != nil {
		return nil, err
	}

	res := s.responses.FindOne(context.Background(), bson.D{{Key: "request_id", Value: objectId}})
	value := &Response{}

	err = res.Decode(value)
	if err != nil {
		return nil, err
	}

	return value, nil
}

func (s *MongoResponseSaver) List(limit int64) ([]*Response, error) {
	ctx := context.Background()

	opts := options.Find().SetLimit(limit).SetSort(bson.D{{Key: "_id", Value: -1}})
	cursor, err := s.responses.Find(ctx, bson.D{}, opts)
	if err != nil {
		return nil, err
	}

	res := make([]*Response, 0, limit/2)

	err = cursor.All(ctx, &res)
	if err != nil {
		return nil, err
	}

	return res, nil
}
