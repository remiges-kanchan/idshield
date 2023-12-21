package groupservice

import (
	"context"
	"fmt"
	"time"

	"github.com/Nerzal/gocloak/v13"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/remiges-tech/alya/router"
	"github.com/remiges-tech/alya/service"
	"github.com/remiges-tech/alya/wscutils"
	"github.com/remiges-tech/logharbour/logharbour"
)

// createCapabilityRequest represents the structure for incoming capability creation requests.
type CreateCapabilityRequest struct {
	Name       *string              `json:"name" validate:"required"`
	Attributes *map[string][]string `json:"attributes,omitempty"`
}

// createCapabilityResponse represents the structure for outgoing capability creation responses.
type CreateCapabilityResponse struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"`
	Path       *string              `json:"path"`
	Attributes *map[string][]string `json:"attributes"`
}

// Capabilities representing Token capabilities.
type Capabilities struct {
	Capability []string `json:"capability"`
}

// HandleCapabilityCreateRequest is a Handler  function for creating capability in keyclock
func HandleCapabilityCreateRequest(c *gin.Context, s *service.Service) {
	lh := s.LogHarbour
	lh.Log("create Capability request received")

	token, err := router.ExtractToken(c.GetHeader("Authorization"))
	if err != nil {
		// Log and respond to token extraction/validation error
		lh.Debug0().LogDebug("Missing or incorrect Authorization header format:", logharbour.DebugInfo{Variables: map[string]any{"error": err}})
		wscutils.SendErrorResponse(c, wscutils.NewErrorResponse("token_missing"))
		return
	}

	// capabilitiesJson := []byte(`{"capability": ["Admin"]}`)

	// isCapable, err := utils.IsCapable(s, token, capabilitiesJson)
	// if err != nil {
	// 	l.LogActivity("Error while decodeing token:", logharbour.DebugInfo{Variables: map[string]interface{}{"error": err}})
	// 	fmt.Println("err", err)
	// 	wscutils.SendErrorResponse(c, wscutils.NewErrorResponse("token_verification_failed"))
	// 	return
	// }

	// if !isCapable {
	// 	l.LogActivity("Unauthorized user:", logharbour.DebugInfo{Variables: map[string]interface{}{"error": err}})
	// 	wscutils.SendErrorResponse(c, wscutils.NewErrorResponse("Unauthorized"))
	// 	return
	// }

	// Unmarshal JSON request into CreateCapabilityRequest struct
	var createCapabilityReq CreateCapabilityRequest

	if err := wscutils.BindJSON(c, &createCapabilityReq); err != nil {
		// Log and respond to JSON Unmarshalling error
		lh.LogActivity("Error Unmarshalling JSON to struct:", logharbour.DebugInfo{Variables: map[string]interface{}{"Error": err.Error()}})
		wscutils.SendErrorResponse(c, wscutils.NewErrorResponse("invalid_json"))
		return
	}

	lh.LogActivity("create capability request parsed", map[string]any{"group": createCapabilityReq.Name})

	//Validate  create cpability request
	validationErrors := validateCreateCapability(createCapabilityReq, c)
	if len(validationErrors) > 0 {

		// Log and respond to validation errors
		lh.Debug0().LogDebug("Validation errors:", logharbour.DebugInfo{Variables: map[string]interface{}{"validationErrors": validationErrors}})
		wscutils.SendErrorResponse(c, wscutils.NewResponse(wscutils.ErrorStatus, nil, validationErrors))
		return
	}

	// Extracting the GoCloak client and realm from the service dependencies
	// for handling authentication and authorization.
	client := s.Dependencies["goclock"].(*gocloak.GoCloak)
	realm := s.Dependencies["realm"].(string)

	// Create a context with a timeout of 10 seconds
	ctx, cancel := context.WithTimeout(c, 10*time.Second)
	defer cancel()

	// Create a new goclock group
	group := gocloak.Group{
		Name:       createCapabilityReq.Name,
		Attributes: createCapabilityReq.Attributes,
	}

	// Create capability
	capabilityCreationID, err := client.CreateGroup(ctx, token, realm, group)
	if err != nil {
		lh.LogActivity("Error while creating capability:", logharbour.DebugInfo{Variables: map[string]interface{}{"error": err}})

		conflictErr := fmt.Sprintf("409 Conflict: Top level group named '%s' already exists.", *createCapabilityReq.Name)

		switch err.Error() {
		case "401 Unauthorized: HTTP 401 Unauthorized":
			lh.Debug0().LogDebug("Unauthorized error occurred: ", logharbour.DebugInfo{Variables: map[string]any{"error": err, "token": token}})
			wscutils.SendErrorResponse(c, wscutils.NewErrorResponse("Unauthorized"))
			return
		case conflictErr:
			lh.Debug0().LogDebug("name conflict error occurred: ", logharbour.DebugInfo{Variables: map[string]interface{}{"error": err}})
			wscutils.SendErrorResponse(c, wscutils.NewErrorResponse("name already exist"))
			return
		default:
			lh.Debug0().LogDebug("Unknown error occurred: ", logharbour.DebugInfo{Variables: map[string]interface{}{"error": err}})
			wscutils.SendErrorResponse(c, wscutils.NewErrorResponse("unknown"))
			return
		}
	}

	// Get a Group Info by using Group ID
	groupInfo, err := client.GetGroup(ctx, token, realm, capabilityCreationID)
	if err != nil {
		fmt.Print(err)
		return
	}

	// Create response struct
	CreateCapabilityResponse := CreateCapabilityResponse{
		ID:         *groupInfo.ID,
		Name:       *groupInfo.Name,
		Path:       groupInfo.Path,
		Attributes: groupInfo.Attributes,
	}
	// Send success response
	wscutils.SendSuccessResponse(c, &wscutils.Response{Data: CreateCapabilityResponse})

	// Log the completion of execution
	lh.LogActivity("Finished execution of createCapability", map[string]string{"Timestamp": time.Now().Format("2006-01-02 15:04:05")})
}

// Validate validates the request body
func validateCreateCapability(req CreateCapabilityRequest, c *gin.Context) []wscutils.ErrorMessage {
	// validate request body using standard validator
	validationErrors := wscutils.WscValidate(req, req.getValsForCreateCapabilityError)

	// add request-specific vals to validation errors
	if len(validationErrors) > 0 {
		return validationErrors
	}
	return validationErrors
}

// getValsForUserError returns a slice of strings to be used as vals for a validation error.
func (req *CreateCapabilityRequest) getValsForCreateCapabilityError(err validator.FieldError) []string {
	var vals []string
	switch err.Field() {
	case "name":
		switch err.Tag() {
		case "required":
			vals = append(vals, "Capability name is required")
			vals = append(vals, *req.Name)
		}

	}
	return vals
}
