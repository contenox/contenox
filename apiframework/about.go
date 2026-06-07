package apiframework

type AboutServer struct {
	Version        string `json:"version"`
	NodeInstanceID string `json:"nodeInstanceID"`
	Tenancy        string `json:"tenancy"`
}
