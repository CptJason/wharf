package controllers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/astaxie/beego"

	"github.com/dockercn/wharf/models"
)

const (
	APIV2ErrorCodeUnknown = iota
	APIV2ErrorCodeUnauthorized
	APIV2ErrorCodeDigestInvalid
	APIV2ErrorCodeSizeInvalid
	APIV2ErrorCodeNameInvalid
	APIV2ErrorCodeTagInvalid
	APIV2ErrorCodeNameUnknown
	APIV2ErrorCodeManifestUnknown
	APIV2ErrorCodeManifestInvalid
	APIV2ErrorCodeManifestUnverified
	APIV2ErrorCodeBlobUnknown
	APIV2ErrorCodeBlobUploadUnknown
)

type V2ErrorDescriptor struct {
	Code        int
	Value       string
	Message     string
	Description string
}

var V2ErrorDescriptors = []V2ErrorDescriptor{
	{
		Code:    APIV2ErrorCodeUnknown,
		Value:   "UNKNOWN",
		Message: "unknown error",
		Description: `Generic error returned when the error does not have an
    API classification.`,
	},
	{
		Code:    APIV2ErrorCodeUnauthorized,
		Value:   "UNAUTHORIZED",
		Message: "access to the requested resource is not authorized",
		Description: `The access controller denied access for the operation on
    a resource. Often this will be accompanied by a 401 Unauthorized
    response status.`,
	},
	{
		Code:    APIV2ErrorCodeDigestInvalid,
		Value:   "DIGEST_INVALID",
		Message: "provided digest did not match uploaded content",
		Description: `When a blob is uploaded, the registry will check that
    the content matches the digest provided by the client. The error may
    include a detail structure with the key "digest", including the
    invalid digest string. This error may also be returned when a manifest
    includes an invalid layer digest.`,
	},
	{
		Code:    APIV2ErrorCodeSizeInvalid,
		Value:   "SIZE_INVALID",
		Message: "provided length did not match content length",
		Description: `When a layer is uploaded, the provided size will be
    checked against the uploaded content. If they do not match, this error
    will be returned.`,
	},
	{
		Code:    APIV2ErrorCodeNameInvalid,
		Value:   "NAME_INVALID",
		Message: "manifest name did not match URI",
		Description: `During a manifest upload, if the name in the manifest
    does not match the uri name, this error will be returned.`,
	},
	{
		Code:    APIV2ErrorCodeTagInvalid,
		Value:   "TAG_INVALID",
		Message: "manifest tag did not match URI",
		Description: `During a manifest upload, if the tag in the manifest
    does not match the uri tag, this error will be returned.`,
	},
	{
		Code:    APIV2ErrorCodeNameUnknown,
		Value:   "NAME_UNKNOWN",
		Message: "repository name not known to registry",
		Description: `This is returned if the name used during an operation is
    unknown to the registry.`,
	},
	{
		Code:    APIV2ErrorCodeManifestUnknown,
		Value:   "MANIFEST_UNKNOWN",
		Message: "manifest unknown",
		Description: `This error is returned when the manifest, identified by
    name and tag is unknown to the repository.`,
	},
	{
		Code:    APIV2ErrorCodeManifestInvalid,
		Value:   "MANIFEST_INVALID",
		Message: "manifest invalid",
		Description: `During upload, manifests undergo several checks ensuring
    validity. If those checks fail, this error may be returned, unless a
    more specific error is included. The detail will contain information
    the failed validation.`,
	},
	{
		Code:    APIV2ErrorCodeManifestUnverified,
		Value:   "MANIFEST_UNVERIFIED",
		Message: "manifest failed signature verification",
		Description: `During manifest upload, if the manifest fails signature
    verification, this error will be returned.`,
	},
	{
		Code:    APIV2ErrorCodeBlobUnknown,
		Value:   "BLOB_UNKNOWN",
		Message: "blob unknown to registry",
		Description: `This error may be returned when a blob is unknown to the
    registry in a specified repository. This can be returned with a
    standard get or if a manifest references an unknown layer during
    upload.`,
	},
	{
		Code:    APIV2ErrorCodeBlobUploadUnknown,
		Value:   "BLOB_UPLOAD_UNKNOWN",
		Message: "blob upload unknown to registry",
		Description: `If a blob upload has been cancelled or was never 
		started, this error code may be returned.`,
	},
}

func manifestsConvertV1(data []byte) error {
	var manifest map[string]interface{}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}

	tag := manifest["tag"]
	namespace, repository := strings.Split(manifest["name"].(string), "/")[0], strings.Split(manifest["name"].(string), "/")[0]

	for k := len(manifest["history"].([]interface{})) - 1; k > 0; k-- {
		v := manifest["history"].([]interface{})[k]
		data := v.(map[string]interface{})["v1Compatibility"].(string)

		var image map[string]interface{}
		if err := json.Unmarshal([]byte(data), &image); err != nil {
			return err
		}

		i := map[string]string{}
		r := new(models.Repository)

		if k == 0 {
			i["Tag"] = tag.(string)
		}
		i["id"] = image["id"].(string)

		//Put V1 JSON
		if err := r.PutJSONFromManifests(i, namespace, repository); err != nil {
			return err
		}

		if k == 0 {
			//Put V1 Tag
			if err := r.PutTagFromManifests(image["id"].(string), namespace, repository, tag.(string), string(data)); err != nil {
				return err
			}
		}

		img := new(models.Image)

		tarsum := manifest["fsLayers"].([]interface{})[k].(map[string]interface{})["blobSum"].(string)
		sha256 := strings.Split(tarsum, ":")[1]

		beego.Debug("[Registry API V2] JSON", v.(map[string]interface{})["v1Compatibility"].(string))

		//Put Image Json
		if err := img.PutJSON(image["id"].(string), v.(map[string]interface{})["v1Compatibility"].(string), models.APIVERSION_V2); err != nil {
			return err
		}

		// //Put Image Layer
		basePath := beego.AppConfig.String("docker::BasePath")
		layerfile := fmt.Sprintf("%v/uuid/%v/layer", basePath, sha256)

		if err := img.PutLayer(image["id"].(string), layerfile, true, int64(image["Size"].(float64))); err != nil {
			return err
		}

		//Put Checksum
		if err := img.PutChecksum(image["id"].(string), sha256, true, ""); err != nil {
			return err
		}

		// //Put Ancestry
		if err := img.PutAncestry(image["id"].(string)); err != nil {
			return err
		}

		beego.Trace("[Registry API V2] Image Tarsum: ", image)
	}

	return nil
}
