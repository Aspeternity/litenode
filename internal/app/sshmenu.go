package app

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"

	"github.com/Aspeternity/litenode/internal/change"
	"github.com/Aspeternity/litenode/internal/ssh"
	"github.com/Aspeternity/litenode/internal/text"
	"github.com/Aspeternity/litenode/internal/ufw"
	"github.com/Aspeternity/litenode/internal/ui"
)

// sshUI holds the SSH menu (DESIGN section 13). It only displays and
// reads input; ssh.Service does the work.
type sshUI struct {
	ctx context.Context
	s   *ui.Session
	svc *ssh.Service
	// reg is the rollback registry, for undoing a change the user rejects.
	reg change.Registry
}

// accountKey is one key in the menu's numbered list.
type accountKey struct {
	account ssh.Account
	key     ssh.Key
}

// admins are root and the sudo users that can log in, as accounts.
func (u *sshUI) admins() []ssh.Account {
	r := u.svc.Confirmer.Manager.Runner
	admins, err := u.svc.Admins(u.ctx)
	if err != nil {
		return nil
	}
	var out []ssh.Account
	for _, a := range admins {
		if acct, err := ssh.LookupAccount(r, a.Name); err == nil {
			out = append(out, acct)
		}
	}
	return out
}

// keys lists the keys of all admins, numbered in this order in the menu.
func (u *sshUI) keys() []accountKey {
	var out []accountKey
	for _, a := range u.admins() {
		keys, _ := u.svc.Keys(a)
		for _, k := range keys {
			out = append(out, accountKey{a, k})
		}
	}
	return out
}

func onOff(b bool) string {
	if b {
		return text.SSHOn
	}
	return text.SSHOff
}

func rootLabel(policy string) string {
	switch policy {
	case ssh.RootYes:
		return text.SSHRootYes
	case ssh.RootNo:
		return text.SSHRootNo
	}
	return text.SSHRootKeys
}

func portList(ports []int) string {
	var s []string
	for _, p := range ports {
		s = append(s, strconv.Itoa(p))
	}
	return strings.Join(s, ",")
}

// sshMainStatus is the SSH value of the main menu status (DESIGN
// section 8), "" if sshd cannot be read.
func sshMainStatus(ctx context.Context, svc *ssh.Service) string {
	e, err := ssh.ReadEffective(ctx, svc.Confirmer.Manager.Runner)
	if err != nil {
		return ""
	}
	return text.SSHMainStatus(portList(e.Ports), e.PubkeyAuth, e.PasswordAuth)
}

// menu is the SSH menu; its status shows what sshd actually uses.
func (u *sshUI) menu() ui.Menu {
	var status []string
	e, err := ssh.ReadEffective(u.ctx, u.svc.Confirmer.Manager.Runner)
	if err != nil {
		status = append(status, u.s.Colors.Bad(text.ActionFailed(err)))
	} else {
		password := onOff(e.PasswordAuth)
		if e.PasswordAuth {
			password = u.s.Colors.Warn(password + " " + text.PasswordWarn)
		}
		status = append(status, text.SSHStatusLine(portList(e.Ports), onOff(e.PubkeyAuth), password, rootLabel(e.PermitRootLogin)))
	}
	keys := u.keys()
	if len(keys) == 0 {
		status = append(status, text.SSHKeysNone)
	} else {
		status = append(status, text.SSHKeysTitle)
		multi := len(u.admins()) > 1
		for i, k := range keys {
			user := ""
			if multi {
				user = k.account.Name
			}
			status = append(status, text.SSHKeyLine(i+1, user, k.key.ShortType(), k.key.Fingerprint(), k.key.Comment))
		}
	}
	return ui.Menu{
		Heading: text.MenuSSH,
		Status:  status,
		Items: []ui.Item{
			{Label: text.SSHKeyAdd, Hint: text.SSHKeyAddHint, Action: u.addKey},
			{Label: text.SSHKeyGen, Hint: text.SSHKeyGenHint, Action: u.generateKey},
			{Label: text.SSHPubkeyToggle, Hint: text.SSHToggleHint, Action: u.togglePubkey},
			{Label: text.SSHPasswordToggle, Hint: text.SSHToggleHint, Action: u.togglePassword},
			{Label: text.SSHRootPolicy, Hint: text.SSHRootHint, Action: u.rootPolicy},
			{Label: text.SSHPortChange, Hint: text.SSHPortHint, Action: u.changePort},
			{Label: text.SSHSudoUser, Action: u.createSudoUser},
			{Label: text.SSHKeyDelete, Hidden: len(keys) == 0, Action: u.deleteKey},
			{Label: text.SSHRollback, Action: u.rollback},
		},
	}
}

// chooseAccount asks which admin a key is for; with root alone it does not
// ask. ok is false when the user went back.
func (u *sshUI) chooseAccount() (ssh.Account, bool, error) {
	admins := u.admins()
	if len(admins) == 0 {
		a, err := ssh.LookupAccount(u.svc.Confirmer.Manager.Runner, "root")
		return a, err == nil, err
	}
	if len(admins) == 1 {
		return admins[0], true, nil
	}
	m := ui.Menu{Heading: text.ChooseAccount}
	for _, a := range admins {
		m.Items = append(m.Items, ui.Item{Label: a.Name})
	}
	choice, err := u.s.Choose(m)
	if err != nil || choice == 0 {
		return ssh.Account{}, false, err
	}
	return admins[choice-1], true, nil
}

func (u *sshUI) addKey() error {
	m := ui.Menu{Heading: text.SSHKeyAdd, Items: []ui.Item{{Label: text.SSHPasteKey}, {Label: text.SSHGitHubImport}}}
	choice, err := u.s.Choose(m)
	if err != nil || choice == 0 {
		return err
	}
	var keys []ssh.Key
	if choice == 1 {
		k, err := u.pasteKey()
		if err != nil || k == nil {
			return err
		}
		keys = []ssh.Key{*k}
	} else {
		keys, err = u.githubKeys()
		if err != nil || len(keys) == 0 {
			return err
		}
	}
	a, ok, err := u.chooseAccount()
	if err != nil || !ok {
		return err
	}
	n, err := u.svc.AddKeys(u.ctx, a, keys)
	if err != nil {
		return err
	}
	if n == 0 {
		u.s.End(text.SSHKeyExists)
		return nil
	}
	u.s.Done(text.KeysAdded(n, len(keys)))
	return nil
}

// pasteKey asks until a valid key is pasted; nil means back.
func (u *sshUI) pasteKey() (*ssh.Key, error) {
	for {
		in, err := u.s.Prompt(text.PromptPublicKey + ": ")
		if err != nil || in == "" {
			return nil, err
		}
		k, err := ssh.ParseKey(in)
		if err == nil {
			return &k, nil
		}
		u.s.Println(u.s.Colors.Warn(describe(err)))
	}
}

// githubKeys downloads a GitHub user's keys and lets the user pick.
func (u *sshUI) githubKeys() ([]ssh.Key, error) {
	for {
		user, err := u.s.Prompt(text.PromptGitHubUser + ": ")
		if err != nil || user == "" {
			return nil, err
		}
		u.s.Println(text.Checking)
		keys, skipped, err := u.svc.GitHubKeys(u.ctx, user)
		if err != nil {
			u.s.Println(u.s.Colors.Warn(describe(err)))
			continue
		}
		u.s.Println(text.GitHubKeysFound(user, len(keys), skipped))
		for i, k := range keys {
			u.s.Println(text.SSHKeyLine(i+1, "", k.ShortType(), k.Fingerprint(), k.Comment))
		}
		for {
			in, err := u.s.Prompt(text.PromptKeyChoice + ": ")
			if err != nil || in == "0" {
				return nil, err
			}
			if in == "" {
				return keys, nil
			}
			if picked, ok := pick(keys, in); ok {
				return picked, nil
			}
			u.s.Println(u.s.Colors.Warn(text.InvalidNumber(1, len(keys))))
		}
	}
}

// pick parses "1,3" into those keys.
func pick(keys []ssh.Key, in string) ([]ssh.Key, bool) {
	var out []ssh.Key
	for _, f := range strings.FieldsFunc(in, func(r rune) bool { return r == ',' || r == ' ' || r == '，' }) {
		n, err := strconv.Atoi(f)
		if err != nil || n < 1 || n > len(keys) {
			return nil, false
		}
		out = append(out, keys[n-1])
	}
	return out, len(out) > 0
}

func (u *sshUI) generateKey() error {
	ok, err := u.s.Confirm(text.ConfirmGenerate)
	if err != nil || !ok {
		return err
	}
	a, ok, err := u.chooseAccount()
	if err != nil || !ok {
		return err
	}
	host, _ := os.Hostname()
	comment, err := u.s.PromptDefault(text.PromptKeyComment, "litenode@"+host)
	if err != nil {
		return err
	}
	k, priv, err := u.svc.GenerateKey(u.ctx, a, comment)
	if err != nil {
		return err
	}
	u.s.Done(text.KeyGenerated)
	u.s.Println("\n" + text.PrivateKeyTitle)
	u.s.Println(strings.TrimRight(string(priv), "\n"))
	u.s.Println(u.s.Colors.Warn(text.PrivateKeyHint))
	u.s.Println("\n" + text.PublicKeyTitle)
	u.s.Println(k.Line())
	return nil
}

// confirmed asks the user to test a new login and reports the outcome
// (awaitConfirm); a kept change closes the flow with done (nothing if
// done is "", for callers with more to do).
func (u *sshUI) confirmed(done string) (bool, error) {
	kept, err := awaitConfirm(u.ctx, u.s, u.svc.Confirmer, u.reg)
	if kept && done != "" {
		u.s.Done(done)
	}
	return kept, err
}

func (u *sshUI) effective() (ssh.Effective, error) {
	return ssh.ReadEffective(u.ctx, u.svc.Confirmer.Manager.Runner)
}

func (u *sshUI) togglePubkey() error {
	e, err := u.effective()
	if err != nil {
		return err
	}
	ok, err := u.s.Confirm(text.ConfirmToggle(text.SSHPubkeyToggle, !e.PubkeyAuth))
	if err != nil || !ok {
		return err
	}
	if err := u.svc.SetPubkeyAuth(u.ctx, !e.PubkeyAuth); err != nil {
		return err
	}
	_, err = u.confirmed(text.ChangeKept)
	return err
}

func (u *sshUI) togglePassword() error {
	e, err := u.effective()
	if err != nil {
		return err
	}
	ok, err := u.s.Confirm(text.ConfirmToggle(text.SSHPasswordToggle, !e.PasswordAuth))
	if err != nil || !ok {
		return err
	}
	if err := u.svc.SetPasswordAuth(u.ctx, !e.PasswordAuth); err != nil {
		return err
	}
	_, err = u.confirmed(text.ChangeKept)
	return err
}

func (u *sshUI) rootPolicy() error {
	e, err := u.effective()
	if err != nil {
		return err
	}
	policies := []string{ssh.RootYes, ssh.RootKeysOnly, ssh.RootNo}
	m := ui.Menu{Heading: text.SSHRootPolicy, Status: []string{text.RootPolicyCurrent(rootLabel(e.PermitRootLogin))}}
	for _, p := range policies {
		m.Items = append(m.Items, ui.Item{Label: rootLabel(p)})
	}
	choice, err := u.s.Choose(m)
	if err != nil || choice == 0 || policies[choice-1] == e.PermitRootLogin {
		return err
	}
	if err := u.svc.SetRootLogin(u.ctx, policies[choice-1]); err != nil {
		return err
	}
	_, err = u.confirmed(text.ChangeKept)
	return err
}

func (u *sshUI) changePort() error {
	old, err := u.svc.CurrentPort(u.ctx)
	if err != nil {
		return err
	}
	port, err := u.s.PromptNumber(text.PromptNewPort, -1, 0, 65535)
	if err != nil || port == 0 {
		return err
	}
	u.s.Println(u.s.Colors.Warn(text.PortChangeNotice(port)))
	ok, err := u.s.Confirm(text.ConfirmPortChange(old, port))
	if err != nil || !ok {
		return err
	}
	if _, err := u.svc.SetPort(u.ctx, port); err != nil {
		return err
	}
	kept, err := u.confirmed("")
	if err != nil || !kept {
		return err
	}
	if err := u.oldPortRules(old); err != nil {
		return err
	}
	u.s.Done(text.PortChanged(port))
	return nil
}

// oldPortRules offers to delete the user's ufw rules for the old port;
// the default is to keep them (D-038).
func (u *sshUI) oldPortRules(old int) error {
	rules, other, err := u.svc.OldPortRules(u.ctx, old)
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		if other > 0 {
			u.s.Println(text.OldPortComplexOnly(old, other))
		}
		return nil
	}
	var names []string
	for _, r := range rules {
		names = append(names, r.Display)
	}
	u.s.Println(text.OldPortRules(old, names, other))
	ok, err := u.s.Confirm(text.ConfirmDeleteOldRules)
	if err != nil || !ok {
		return err
	}
	for _, r := range rules {
		if err := ufw.DeleteUserRule(u.ctx, u.svc.Confirmer.Manager.Runner, r); err != nil {
			return err
		}
	}
	u.s.Println(text.OldRulesDeleted)
	return nil
}

func (u *sshUI) createSudoUser() error {
	var name string
	for {
		in, err := u.s.Prompt(text.PromptSudoName + ": ")
		if err != nil || in == "" {
			return err
		}
		if !ssh.ValidUserName(in) {
			u.s.Println(u.s.Colors.Warn(text.ErrUserName))
			continue
		}
		name = in
		break
	}
	m := ui.Menu{Heading: text.ChoosePassword, Items: []ui.Item{{Label: text.SudoPasswordGen}, {Label: text.SudoPasswordInput}}}
	choice, err := u.s.Choose(m)
	if err != nil || choice == 0 {
		return err
	}
	var password string
	generated := choice == 1
	if generated {
		if password, err = ssh.GeneratePassword(u.svc.Rand); err != nil {
			return err
		}
	} else if password, err = u.askPassword(); err != nil || password == "" {
		return err
	}
	var keys []ssh.Key
	if root, err := ssh.LookupAccount(u.svc.Confirmer.Manager.Runner, "root"); err == nil {
		if rootKeys, _ := u.svc.Keys(root); len(rootKeys) > 0 {
			ok, err := u.s.Confirm(text.ConfirmCopyRootKeys(len(rootKeys), name))
			if err != nil {
				return err
			}
			if ok {
				keys = rootKeys
			}
		}
	}
	if err := u.svc.CreateSudoUser(u.ctx, name, password, keys); err != nil {
		return err
	}
	u.s.Done(text.SudoUserCreated(name))
	if generated {
		u.s.Println(u.s.Colors.Warn(text.GeneratedPassword(name, password)))
	}
	return nil
}

// askPassword reads a password twice without echo; "" means back.
func (u *sshUI) askPassword() (string, error) {
	for {
		p, err := u.s.PromptSecret(text.PromptPassword + ": ")
		if err != nil || p == "" {
			return "", err
		}
		if err := ssh.CheckPassword(p); err != nil {
			u.s.Println(u.s.Colors.Warn(describe(err)))
			continue
		}
		again, err := u.s.PromptSecret(text.PromptPassword2 + ": ")
		if err != nil {
			return "", err
		}
		if again == p {
			return p, nil
		}
		u.s.Println(u.s.Colors.Warn(text.PasswordMismatch))
	}
}

func (u *sshUI) deleteKey() error {
	keys := u.keys()
	i, err := u.s.PromptNumber(text.PromptKeyNumber, -1, 0, len(keys))
	if err != nil || i == 0 {
		return err
	}
	k := keys[i-1]
	ok, err := u.s.Confirm(text.ConfirmDeleteKey(k.account.Name, k.key.Fingerprint()))
	if err != nil || !ok {
		return err
	}
	if err := u.svc.DeleteKey(u.ctx, k.account, k.key.Fingerprint()); err != nil {
		return err
	}
	_, err = u.confirmed(text.ChangeKept)
	return err
}

func (u *sshUI) rollback() error {
	ok, err := u.s.Confirm(text.ConfirmRollback)
	if err != nil || !ok {
		return err
	}
	if _, err := u.svc.RollBackLast(u.ctx); err != nil {
		return err
	}
	_, err = u.confirmed(text.ChangeKept)
	return err
}

// describeSSH maps SSH errors to text; ok is false for other errors.
func describeSSH(err error) (string, bool) {
	var pt *ssh.PortTakenError
	for target, msg := range map[error]string{
		ssh.ErrNoInclude: text.ErrSSHNoInclude, ssh.ErrIncludeLate: text.ErrSSHIncludeLate,
		ssh.ErrNotEffective: text.ErrSSHNotEffective, ssh.ErrNeedPasswordLogin: text.ErrNeedPasswordLogin,
		ssh.ErrNeedKeyLogin: text.ErrNeedKeyLogin, ssh.ErrNeedKey: text.ErrNeedKey,
		ssh.ErrNeedSudoUser: text.ErrNeedSudoUser, ssh.ErrLockout: text.ErrSSHLockout,
		ssh.ErrLastKey: text.ErrLastKey, ssh.ErrKeyFormat: text.ErrKeyFormat, ssh.ErrKeyType: text.ErrKeyType,
		ssh.ErrKeyWeak: text.ErrKeyWeak, ssh.ErrGitHubUser: text.ErrGitHubUser, ssh.ErrGitHubNone: text.ErrGitHubNone,
		ssh.ErrPortRange: text.ErrSSHPortRange, ssh.ErrPortSame: text.ErrSSHPortSame, ssh.ErrUserName: text.ErrUserName,
		ssh.ErrUserExists: text.ErrUserExists, ssh.ErrPasswordShort: text.ErrPasswordShort,
		ssh.ErrPasswordChars: text.ErrPasswordChars, ssh.ErrNothingToRollBack: text.ErrNothingToRollBack,
	} {
		if errors.Is(err, target) {
			return msg, true
		}
	}
	if errors.As(err, &pt) {
		return portConflicts(pt.Conflicts), true
	}
	return "", false
}
