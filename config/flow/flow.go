package flow

import (
	"encoding/json"
	"fmt"

	legacyDef "github.com/TIBCOSoftware/flogo-contrib/action/flow/definition"
	legacyData "github.com/TIBCOSoftware/flogo-lib/core/data"
	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/coerce"
	"github.com/project-flogo/core/data/metadata"
	"github.com/project-flogo/flow/definition"
	"github.com/project-flogo/legacybridge/config"
)

func convertLegacyFlow(rep *legacyDef.DefinitionRep) (*definition.DefinitionRep, error) {

	if rep.RootTask != nil {
		return nil, fmt.Errorf("definition too old to be automatically converted")
	}

	newDef := &definition.DefinitionRep{}
	newDef.Name = rep.Name
	newDef.ModelID = rep.ModelID
	newDef.ExplicitReply = rep.ExplicitReply

	if rep.Metadata != nil {
		newDef.Metadata = &metadata.IOMetadata{}
		if len(rep.Metadata.Input) > 0 {
			newDef.Metadata.Input = make(map[string]data.TypedValue)
			for name, attr := range rep.Metadata.Input {
				newAttr, err := config.ConvertLegacyAttr(attr)
				if err != nil {
					return nil, err
				}
				newDef.Metadata.Input[name] = newAttr
			}
		}
		if len(rep.Metadata.Output) > 0 {
			newDef.Metadata.Output = make(map[string]data.TypedValue)
			for name, attr := range rep.Metadata.Output {
				newAttr, err := config.ConvertLegacyAttr(attr)
				if err != nil {
					return nil, err
				}
				newDef.Metadata.Output[name] = newAttr
			}
		}
	}

	if len(rep.Tasks) != 0 {

		for _, taskRep := range rep.Tasks {

			task, err := createTask(taskRep)

			if err != nil {
				return nil, err
			}
			newDef.Tasks = append(newDef.Tasks, task)
		}
	}

	if len(rep.Links) != 0 {

		for _, linkRep := range rep.Links {

			link := createLink(linkRep)
			newDef.Links = append(newDef.Links, link)
		}
	}

	if rep.ErrorHandler != nil {

		errorHandler := &definition.ErrorHandlerRep{}
		newDef.ErrorHandler = errorHandler

		if len(rep.ErrorHandler.Tasks) != 0 {

			for _, taskRep := range rep.ErrorHandler.Tasks {

				task, err := createTask(taskRep)
				if err != nil {
					return nil, err
				}
				errorHandler.Tasks = append(errorHandler.Tasks, task)
			}
		}

		if len(rep.ErrorHandler.Links) != 0 {

			for _, linkRep := range rep.ErrorHandler.Links {
				link := createLink(linkRep)
				errorHandler.Links = append(errorHandler.Links, link)
			}
		}
	}

	return newDef, nil
}

func createTask(rep *legacyDef.TaskRep) (*definition.TaskRep, error) {
	task := &definition.TaskRep{}
	task.ID = rep.ID
	task.Name = rep.Name
	task.Settings = rep.Settings
	task.Type = rep.Type

	if rep.ActivityCfgRep != nil {

		actCfg, err := createActivityConfig(rep.ActivityCfgRep)
		if err != nil {
			return nil, err
		}

		task.ActivityCfgRep = actCfg
	}

	return task, nil
}

func createActivityConfig(rep *legacyDef.ActivityConfigRep) (*activity.Config, error) {

	activityCfg := &activity.Config{}
	activityCfg.Settings = rep.Settings

	activityCfg.Ref = rep.Ref
	settings, _ := config.ConvertValues(rep.Settings)
	input, inputSchemas := config.ConvertValues(rep.InputAttrs)
	output, outputSchemas := config.ConvertValues(rep.OutputAttrs)

	if rep.Mappings != nil {
		lm := &legacyData.IOMappings{}
		lm.Input = rep.Mappings.Input
		lm.Output = rep.Mappings.Output

		inputMappings, outputMappings, err := config.ConvertLegacyMappings(lm, definition.GetDataResolver())
		if err != nil {
			return nil, err
		}

		if len(inputMappings) > 0 {
			for key, value := range inputMappings {
				input[key] = value
			}
		}

		if len(outputMappings) > 0 {
			for key, value := range outputMappings {
				output[key] = value
			}
		}
	}

	if len(settings) > 0 {
		activityCfg.Settings = settings
	}

	ok, err := upgradeReturnReply(input, rep, activityCfg)
	if err != nil {
		return nil, fmt.Errorf("upgrade %s error %s", rep.Ref, err.Error())
	}
	if !ok {
		if len(input) > 0 {
			activityCfg.Input = input
		}
	}

	if len(output) > 0 {
		activityCfg.Output = output
	}

	if len(inputSchemas) > 0 || len(outputSchemas) > 0 {
		activityCfg.Schemas = &activity.SchemaConfig{}

		if len(inputSchemas) > 0 {
			activityCfg.Schemas.Input = inputSchemas
		}

		if len(outputSchemas) > 0 {
			activityCfg.Schemas.Output = outputSchemas
		}
	}

	return activityCfg, nil
}

func createLink(linkRep *legacyDef.LinkRep) *definition.LinkRep {

	link := &definition.LinkRep{}
	link.Name = linkRep.Name
	link.Value = linkRep.Value
	link.Type = linkRep.Type
	link.ToID = linkRep.ToID
	link.FromID = linkRep.FromID

	return link
}

func upgradeReturnReply(input map[string]interface{}, rep *legacyDef.ActivityConfigRep, conf *activity.Config) (bool, error) {
	var isReturnReply bool
	if rep.Ref == "github.com/TIBCOSoftware/flogo-contrib/activity/actreturn" {
		conf.Ref = "github.com/project-flogo/contrib/activity/actreturn"
		isReturnReply = true
	}

	if rep.Ref == "github.com/TIBCOSoftware/flogo-contrib/activity/actreply" {
		conf.Ref = "github.com/project-flogo/contrib/activity/actreply"
		isReturnReply = true
	}

	if isReturnReply {
		if input["mappings"] != nil {
			bytes, err := coerce.ToBytes(input["mappings"])
			if err != nil {
				return false, err
			}
			var returnMapping []*legacyData.MappingDef
			err = json.Unmarshal(bytes, &returnMapping)
			if err != nil {
				return false, err
			}
			input, err = config.HandleMappings(returnMapping, definition.GetDataResolver())
			if err != nil {
				return false, err
			}

			if len(conf.Settings) > 0 {
				conf.Settings["mappings"] = input
			} else {
				settingMap := make(map[string]interface{})
				settingMap["mappings"] = input
				conf.Settings = settingMap
			}
		}
		return true, nil
	}

	return false, nil

}
