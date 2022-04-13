package callbackfixtures

const (
	PersistentDataStorePathInit          = "/init"
	PersistentDataStorePathGet           = "/get"
	PersistentDataStorePathGetAll        = "/getAll"
	PersistentDataStorePathUpsert        = "/upsert"
	PersistentDataStorePathIsInitialized = "/initialized"
)

type DataStoreCollection struct {
	Kind  string               `json:"kind"`
	Items []DataStoreKeyedItem `json:"items"`
}

type DataStoreKeyedItem struct {
	Key  string                  `json:"key"`
	Item DataStoreSerializedItem `json:"item"`
}

type DataStoreSerializedItem struct {
	Version int    `json:"version"`
	Data    string `json:"data"`
}

type DataStoreInitParams struct {
	Data []DataStoreCollection `json:"data"`
}

type DataStoreGetParams struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
}

type DataStoreGetResponse struct {
	Item *DataStoreSerializedItem `json:"item"`
}

type DataStoreGetAllParams struct {
	Kind string `json:"kind"`
}

type DataStoreGetAllResponse struct {
	Items []DataStoreKeyedItem `json:"items"`
}

type DataStoreUpsertParams struct {
	Kind string                  `json:"kind"`
	Key  string                  `json:"key"`
	Item DataStoreSerializedItem `json:"item"`
}

type DataStoreUpsertResponse struct {
	Updated bool `json:"updated"`
}

type DataStoreIsInitializedResponse struct {
	Result bool `json:"result"`
}
