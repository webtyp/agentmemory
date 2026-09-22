package agentmemory

import "webtyp.com/model"

var MessageModel = model.Definition{
	Name: "message",
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "session_id", Type: model.Text()},
		{Name: "role", Type: model.Text()},
		{Name: "content", Type: model.Text()},
		{Name: "tool_name", Type: model.Text(), OmitEmpty: true},
		{Name: "tool_call_id", Type: model.Text(), OmitEmpty: true},
		{Name: "tool_calls", Type: model.Text(), OmitEmpty: true},
		{Name: "token_count", Type: model.Int()},
		{Name: "created_at", Type: model.Int()},
	},
}

var EpisodeModel = model.Definition{
	Name: "episode",
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "session_id", Type: model.Text()},
		{Name: "summary", Type: model.Text()},
		{Name: "token_count", Type: model.Int()},
		{Name: "from_msg_id", Type: model.Text()},
		{Name: "to_msg_id", Type: model.Text()},
		{Name: "created_at", Type: model.Int()},
	},
}

var ToolLogModel = model.Definition{
	Name: "tool_log",
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "session_id", Type: model.Text()},
		{Name: "tool_name", Type: model.Text()},
		{Name: "input_json", Type: model.Text()},
		{Name: "output_text", Type: model.Text(), OmitEmpty: true},
		{Name: "err_text", Type: model.Text(), OmitEmpty: true},
		{Name: "duration_ms", Type: model.Int()},
		{Name: "created_at", Type: model.Int()},
	},
}
