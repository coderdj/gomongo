package main
import (
	"context"
	"fmt"
	"encoding/json"
	"errors"
	"net/http"
	"time"
	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
)

func GetCommandEndpoint(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("content-type", "application/json")
	params := mux.Vars(request)
	detector := params["detector"]

	control_doc, err := GetControlDoc(detector)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(`{"message": "` + err.Error() + `"}`))
		return
	}
	json.NewEncoder(response).Encode(control_doc)
	return
}

func UpdateCommandEndpoint(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("content-type", "application/json")
	if err := request.ParseForm(); err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(`{"message": "Malformed request error: ` + err.Error() + `"}`))
		return
	}

	// Right now we ONLY support controlling the TPC. Fail if not TPC.
	// Probably if you're reading this you want to make it support the other detectors,
	// so right here is a good place to start. :-)
	params := mux.Vars(request)
	detector := params["detector"]
	if detector != "tpc" {
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(`{"message": "Sorry, we don't support detector` +
			detector + ` yet!"}`))
		return
	}	

	// As a precursor to doing anything the TPC DAQ must be IDLE and the current command
	// must have it 'deactivated'. So let's have a look then. First the control doc.
	// It must also be in 'remote' mode
	control_doc, err := GetControlDoc(detector)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(
			`{"message": "` + err.Error() + `"}`))
		return
	}
	if( (control_doc.Remote != "true"){
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(
			`{"message": "TPC must be in remote control mode ` +
				`to control via API"}`))
		return
	}
		
	if( (control_doc.Active != "false" &&  request.FormValue("active") == "true") ||
		(control_doc.Active != "true" && request.FormValue("active") == "false") ||
		control_doc.LinkMV != "false" ||
		control_doc.LinkNV != "false"){
		fmt.Println(control_doc)
		fmt.Println(control_doc.Active)
		fmt.Println(control_doc.LinkMV)
		fmt.Println(control_doc.LinkNV)
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(
			`{"message": "TPC must be inactive and unlinked to other ` +
				` detectors to control via API"}`))
		return
	}
	// Now the status
	detector_status, err := GetDetectorStatus(detector, -1)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(`{"message": "`+err.Error()+`"}`))
		return
	}
	if detector_status.Status != 0 && request.FormValue("active") != "false" {
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(`{"message": "Detector ` + detector +
			` must be IDLE (0) but it is ` +
			fmt.Sprint(detector_status.Status) + `"}`))
		return
	}

	// The prerequisites are met, so now we can validate the incoming command
	// to make sure it's got everything it needs
	var new_control Control
	new_control.Active = request.FormValue("active")
	new_control.Mode = request.FormValue("mode")
	new_control.Comment = request.FormValue("comment")
	new_control.StopAfter = request.FormValue("stop_after")

	// Do update
	err = UpdateControlDoc(new_control, request.FormValue("api_user"), detector);
	if err == nil {
		response.WriteHeader(http.StatusOK)
		response.Write([]byte(`{"message": "Update success!"}`))
		return
	}
	response.WriteHeader(http.StatusInternalServerError)
	response.Write([]byte(`{"message": "Error updating mongo: `+err.Error()+`"}`))
	return
}


func UpdateControlDoc(new_control Control, user string, detector string) (error){
	// Case 1: set inactive. Ignore everything else and just update to inactive.
	// Case 2: set active. Allow the following fields: (mode, stop_after, comment)
	//         And fix the following: LinkMV(false), LinkNV(false)
	//         Check options DB for 'mode'
	// All cases: set User to requesting API user
	collection := client.Database("daq").Collection("detector_control")
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)

	if new_control.Active == "false" {
		_, err := collection.UpdateOne(
			context.Background(),
			bson.M{"detector": detector},
			bson.M{"$set": bson.M{"active": "false", "user": user}},
		)
		return err
	}

	// Look up options
	options_collection := client.Database("daq").Collection("options")
	cursor, err := options_collection.Find(ctx, bson.M{"name": new_control.Mode})
	if err != nil{
		return err
	}
	// There is no 'count'. Amazing driver.
	cursorempty := true
	for cursor.Next(ctx){
		cursorempty = false
	}
	if cursorempty {
		return errors.New("There is no options doc by the name " + new_control.Mode)
	}
	// Probably need some check if Comment and StopAfter are included here.

	// Now we can update to start the DAQ
	_, err = collection.UpdateOne(
		context.Background(),
		bson.M{"detector": detector},
		bson.M{"$set": bson.M{"active": "true", "mode": new_control.Mode,
			"user": user, "link_nv": "false", "link_mv": "false",
			"comment": new_control.Comment, "stop_after": new_control.StopAfter}},
	)
	if err != nil{
		return err
	}
	return nil
}

func GetControlDoc(detector string) (Control, error){
	// Just fetches the control doc for this detector
	collection := client.Database("daq").Collection("detector_control")
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	cursor, err := collection.Find(ctx, bson.M{"detector": detector})
	var control_doc Control
	if err != nil {
		return control_doc, err
	}
	for cursor.Next(ctx) {
		cursor.Decode(&control_doc)
		return control_doc, nil;
	}
	return control_doc, errors.New("No control document found for detector " + detector);
}
