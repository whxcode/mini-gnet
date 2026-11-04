package command

import (
	"strings"
	"testing"
)

// TestLookupCommand 验证主循环能拿到命令与参数，普通提问不被识别为命令。
func TestLookupCommand(t *testing.T) {
	for _, tc := range []struct {
		text    string
		name    Name
		args    string
		wantErr bool
	}{
		{"/help", HelpName, "", false},
		{"/skills", SkillsName, "", false},
		{"/skills code-review", SkillsName, "code-review", false},
		{"/skills --clear", SkillsName, "--clear", false},
		{"/skills one two", "", "", true},
		{"/config", ConfigName, "", false},
		{"/new", NewName, "", false},
		{"/exit", ExitName, "", false},
		{"/resume", ResumeName, "", false},
		{"/ps", PSName, "", false},
		{"/ps 2", PSName, "2", false},
		{"/ps 1 2", "", "", true},
		{"  /resume\told-id  ", ResumeName, "old-id", false},
		{"/unknown", "", "", true},
		{"/help unexpected", "", "", true},
		{"/config unexpected", "", "", true},
		{"/new unexpected", "", "", true},
		{"/exit unexpected", "", "", true},
		{"/resume first second", "", "", true},
		{"/Users/mac/project 下有什么", "", "", false},
		{"普通问题", "", "", false},
		{"/help\n这是一段代码", "", "", false},
		{"", "", "", false},
	} {
		cmd, args, err := Lookup(tc.text)
		if (err != nil) != tc.wantErr {
			t.Errorf("%q: err=%v, wantErr=%v", tc.text, err, tc.wantErr)
		}

		if tc.name == "" {
			if cmd != nil {
				t.Errorf("%q 不应找到命令", tc.text)
			}

			continue
		}

		if cmd == nil || cmd.Name != tc.name || cmd.Handler == nil || strings.Join(args, " ") != tc.args {
			t.Errorf("%q: cmd=%+v, args=%v", tc.text, cmd, args)
		}
	}
}
