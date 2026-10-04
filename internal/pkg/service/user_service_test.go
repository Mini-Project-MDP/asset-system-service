package service

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/jwt"
)

// fakeUserAdminRepository is an in-memory domain.UserAdminRepository.
type fakeUserAdminRepository struct {
	users  map[string]*domain.UserProfile
	hashes map[string]string
	roles  map[string]*domain.RoleDto
	nextID int
}

func newFakeUserAdminRepository() *fakeUserAdminRepository {
	role := func(id, code string, active bool) *domain.RoleDto {
		return &domain.RoleDto{ID: id, Code: code, Name: code, IsActive: active}
	}
	return &fakeUserAdminRepository{
		users:  map[string]*domain.UserProfile{},
		hashes: map[string]string{},
		roles: map[string]*domain.RoleDto{
			"role_master": role("role_master", "MASTER_ADMIN", true),
			"role_mgr":    role("role_mgr", "ASSET_MANAGER", true),
			"role_sa":     role("role_sa", "SA", true),
			"role_sd":     role("role_sd", "SD", true),
			"role_off":    role("role_off", "SS", false),           // a standard role that is deactivated
			"role_user":   role("role_user", "REGULAR_USER", true), // legacy, not one of the 8
			"role_cabang": role("role_cabang", "Cabang", true),     // label only, not a login role
		},
	}
}

func (f *fakeUserAdminRepository) GetUser(ctx context.Context, id string) (*domain.UserProfile, error) {
	if u, ok := f.users[id]; ok {
		c := *u
		return &c, nil
	}
	return nil, nil
}

func (f *fakeUserAdminRepository) GetRole(ctx context.Context, id string) (*domain.RoleDto, error) {
	if r, ok := f.roles[id]; ok {
		c := *r
		return &c, nil
	}
	return nil, nil
}

func (f *fakeUserAdminRepository) duplicate(in domain.SaveUserInput, exceptID string) bool {
	for id, u := range f.users {
		if id != exceptID && (u.Email == in.Email || u.EmployeeNo == in.EmployeeNo) {
			return true
		}
	}
	return false
}

func (f *fakeUserAdminRepository) CreateUser(ctx context.Context, in domain.SaveUserInput, passwordHash, status string) (string, error) {
	if f.duplicate(in, "") {
		return "", domain.ErrDuplicate
	}
	f.nextID++
	id := "usr" + strconv.Itoa(f.nextID)
	f.users[id] = &domain.UserProfile{
		ID: id, Name: in.Name, Email: in.Email, EmployeeNo: in.EmployeeNo, Status: status,
		Roles: []domain.RoleDto{*f.roles[in.RoleID]},
	}
	f.hashes[id] = passwordHash
	return id, nil
}

func (f *fakeUserAdminRepository) UpdateUser(ctx context.Context, id string, in domain.SaveUserInput, status string) error {
	if f.duplicate(in, id) {
		return domain.ErrDuplicate
	}
	f.users[id] = &domain.UserProfile{
		ID: id, Name: in.Name, Email: in.Email, EmployeeNo: in.EmployeeNo, Status: status,
		Roles: []domain.RoleDto{*f.roles[in.RoleID]},
	}
	return nil
}

func (f *fakeUserAdminRepository) CountOtherActiveAdmins(ctx context.Context, excludeUserID string) (int, error) {
	n := 0
	for id, u := range f.users {
		if id == excludeUserID || u.Status != domain.UserStatusActive {
			continue
		}
		for _, r := range u.Roles {
			if r.Code == "MASTER_ADMIN" {
				n++
			}
		}
	}
	return n, nil
}

func validUser() domain.SaveUserInput {
	return domain.SaveUserInput{Name: "Laras P.", Email: "Laras@Company.co", RoleID: "role_sa"}
}

func TestCreateUser(t *testing.T) {
	ctx := context.Background()

	t.Run("creates an active user with normalized email and default employee_no", func(t *testing.T) {
		svc := NewUserService(newFakeUserAdminRepository())
		got, err := svc.CreateUser(ctx, validUser())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Email != "laras@company.co" || got.EmployeeNo != "laras@company.co" || got.Status != domain.UserStatusActive {
			t.Fatalf("got %+v, want lowercased email, employee_no defaulted to email, ACTIVE", got)
		}
		if len(got.Roles) != 1 || got.Roles[0].Code != "SA" {
			t.Fatalf("got roles %+v, want single SA role", got.Roles)
		}
	})

	t.Run("keeps a supplied employee_no", func(t *testing.T) {
		svc := NewUserService(newFakeUserAdminRepository())
		in := validUser()
		in.EmployeeNo = " EMP100 "
		got, err := svc.CreateUser(ctx, in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.EmployeeNo != "EMP100" {
			t.Fatalf("employee_no = %q, want EMP100", got.EmployeeNo)
		}
	})

	t.Run("hashes a supplied password", func(t *testing.T) {
		repo := newFakeUserAdminRepository()
		svc := NewUserService(repo)
		in := validUser()
		in.Password = "Sup3rSecret!"
		got, err := svc.CreateUser(ctx, in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !jwt.CheckPasswordHash("Sup3rSecret!", repo.hashes[got.ID]) {
			t.Fatal("stored hash does not match the supplied password")
		}
	})

	t.Run("stores an unguessable password when none is supplied", func(t *testing.T) {
		repo := newFakeUserAdminRepository()
		svc := NewUserService(repo)
		got, err := svc.CreateUser(ctx, validUser())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if repo.hashes[got.ID] == "" {
			t.Fatal("expected a generated password hash, got empty")
		}
	})

	t.Run("rejects invalid input", func(t *testing.T) {
		svc := NewUserService(newFakeUserAdminRepository())
		cases := map[string]func(*domain.SaveUserInput){
			"empty name":        func(i *domain.SaveUserInput) { i.Name = "  " },
			"empty email":       func(i *domain.SaveUserInput) { i.Email = "" },
			"malformed email":   func(i *domain.SaveUserInput) { i.Email = "not-an-email" },
			"email with name":   func(i *domain.SaveUserInput) { i.Email = "Laras <laras@company.co>" },
			"missing role":      func(i *domain.SaveUserInput) { i.RoleID = "" },
			"unknown role":      func(i *domain.SaveUserInput) { i.RoleID = "ghost" },
			"legacy role":       func(i *domain.SaveUserInput) { i.RoleID = "role_user" },
			"Cabang label role": func(i *domain.SaveUserInput) { i.RoleID = "role_cabang" },
			"inactive role":     func(i *domain.SaveUserInput) { i.RoleID = "role_off" },
			"short password":    func(i *domain.SaveUserInput) { i.Password = "short" },
		}
		for name, mutate := range cases {
			in := validUser()
			mutate(&in)
			if _, err := svc.CreateUser(ctx, in); !errors.Is(err, ErrUserValidation) {
				t.Errorf("%s: err = %v, want ErrUserValidation", name, err)
			}
		}
	})

	t.Run("duplicate email is a conflict", func(t *testing.T) {
		svc := NewUserService(newFakeUserAdminRepository())
		if _, err := svc.CreateUser(ctx, validUser()); err != nil {
			t.Fatalf("first create: %v", err)
		}
		if _, err := svc.CreateUser(ctx, validUser()); !errors.Is(err, ErrUserConflict) {
			t.Fatalf("second create err = %v, want ErrUserConflict", err)
		}
	})

	t.Run("accepts all eight standard roles", func(t *testing.T) {
		repo := newFakeUserAdminRepository()
		for id, code := range map[string]string{
			"r1": "SS", "r2": "RSM", "r3": "GRSM", "r4": "NSM",
		} {
			repo.roles[id] = &domain.RoleDto{ID: id, Code: code, IsActive: true}
		}
		svc := NewUserService(repo)
		for i, id := range []string{"role_master", "role_mgr", "role_sa", "role_sd", "r1", "r2", "r3", "r4"} {
			in := validUser()
			in.Email = "u" + strconv.Itoa(i) + "@company.co"
			in.RoleID = id
			if _, err := svc.CreateUser(ctx, in); err != nil {
				t.Errorf("role %s: unexpected error: %v", id, err)
			}
		}
	})
}

func TestUpdateUser(t *testing.T) {
	ctx := context.Background()

	setup := func() (domain.UserService, *fakeUserAdminRepository, *domain.UserProfile) {
		repo := newFakeUserAdminRepository()
		svc := NewUserService(repo)
		in := validUser()
		in.EmployeeNo = "EMP100"
		u, err := svc.CreateUser(ctx, in)
		if err != nil {
			panic(err)
		}
		return svc, repo, u
	}

	t.Run("unknown id is not found", func(t *testing.T) {
		svc := NewUserService(newFakeUserAdminRepository())
		if _, err := svc.UpdateUser(ctx, "nope", validUser()); !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("err = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("changes name, email and role; keeps employee_no when omitted", func(t *testing.T) {
		svc, _, u := setup()
		got, err := svc.UpdateUser(ctx, u.ID, domain.SaveUserInput{Name: "Laras Putri", Email: "laras.p@company.co", RoleID: "role_sd"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name != "Laras Putri" || got.Email != "laras.p@company.co" || got.EmployeeNo != "EMP100" || got.Roles[0].Code != "SD" {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("nil IsActive keeps status; false deactivates", func(t *testing.T) {
		svc, _, u := setup()
		got, _ := svc.UpdateUser(ctx, u.ID, validUser())
		if got.Status != domain.UserStatusActive {
			t.Fatalf("status = %s, want ACTIVE", got.Status)
		}
		off := false
		in := validUser()
		in.IsActive = &off
		got, err := svc.UpdateUser(ctx, u.ID, in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Status != domain.UserStatusInactive {
			t.Fatalf("status = %s, want INACTIVE", got.Status)
		}
	})

	t.Run("email taken by another user is a conflict", func(t *testing.T) {
		svc, _, _ := setup()
		other := validUser()
		other.Email = "other@company.co"
		o, _ := svc.CreateUser(ctx, other)
		in := validUser()
		in.Email = "laras@company.co"
		if _, err := svc.UpdateUser(ctx, o.ID, in); !errors.Is(err, ErrUserConflict) {
			t.Fatalf("err = %v, want ErrUserConflict", err)
		}
	})

	t.Run("password cannot be changed through update", func(t *testing.T) {
		svc, _, u := setup()
		in := validUser()
		in.Password = "NewPassw0rd!"
		if _, err := svc.UpdateUser(ctx, u.ID, in); !errors.Is(err, ErrUserValidation) {
			t.Fatalf("err = %v, want ErrUserValidation", err)
		}
	})

	t.Run("the last active admin cannot be demoted or deactivated", func(t *testing.T) {
		repo := newFakeUserAdminRepository()
		svc := NewUserService(repo)
		admin, _ := svc.CreateUser(ctx, domain.SaveUserInput{Name: "Andi", Email: "andi@company.co", RoleID: "role_master"})

		demote := domain.SaveUserInput{Name: "Andi", Email: "andi@company.co", RoleID: "role_sa"}
		if _, err := svc.UpdateUser(ctx, admin.ID, demote); !errors.Is(err, ErrLastAdmin) {
			t.Fatalf("demote err = %v, want ErrLastAdmin", err)
		}
		off := false
		deactivate := domain.SaveUserInput{Name: "Andi", Email: "andi@company.co", RoleID: "role_master", IsActive: &off}
		if _, err := svc.UpdateUser(ctx, admin.ID, deactivate); !errors.Is(err, ErrLastAdmin) {
			t.Fatalf("deactivate err = %v, want ErrLastAdmin", err)
		}
	})

	t.Run("an admin can be demoted while another active admin exists", func(t *testing.T) {
		repo := newFakeUserAdminRepository()
		svc := NewUserService(repo)
		a, _ := svc.CreateUser(ctx, domain.SaveUserInput{Name: "Andi", Email: "andi@company.co", RoleID: "role_master"})
		svc.CreateUser(ctx, domain.SaveUserInput{Name: "Budi", Email: "budi@company.co", RoleID: "role_master"})
		if _, err := svc.UpdateUser(ctx, a.ID, domain.SaveUserInput{Name: "Andi", Email: "andi@company.co", RoleID: "role_sa"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("an inactive second admin does not count as a spare admin", func(t *testing.T) {
		repo := newFakeUserAdminRepository()
		svc := NewUserService(repo)
		a, _ := svc.CreateUser(ctx, domain.SaveUserInput{Name: "Andi", Email: "andi@company.co", RoleID: "role_master"})
		off := false
		svc.CreateUser(ctx, domain.SaveUserInput{Name: "Budi", Email: "budi@company.co", RoleID: "role_master", IsActive: &off})
		if _, err := svc.UpdateUser(ctx, a.ID, domain.SaveUserInput{Name: "Andi", Email: "andi@company.co", RoleID: "role_sa"}); !errors.Is(err, ErrLastAdmin) {
			t.Fatalf("err = %v, want ErrLastAdmin", err)
		}
	})
}

func TestAssignRoles(t *testing.T) {
	ctx := context.Background()

	t.Run("sets exactly one role", func(t *testing.T) {
		svc := NewUserService(newFakeUserAdminRepository())
		u, _ := svc.CreateUser(ctx, validUser())
		got, err := svc.AssignRoles(ctx, u.ID, []string{"role_mgr"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got.Roles) != 1 || got.Roles[0].Code != "ASSET_MANAGER" {
			t.Fatalf("got roles %+v, want ASSET_MANAGER", got.Roles)
		}
	})

	t.Run("rejects zero or several roles", func(t *testing.T) {
		svc := NewUserService(newFakeUserAdminRepository())
		u, _ := svc.CreateUser(ctx, validUser())
		for _, ids := range [][]string{nil, {}, {""}, {"role_mgr", "role_sd"}} {
			if _, err := svc.AssignRoles(ctx, u.ID, ids); !errors.Is(err, ErrUserValidation) {
				t.Errorf("ids %v: err = %v, want ErrUserValidation", ids, err)
			}
		}
	})

	t.Run("unknown user is not found", func(t *testing.T) {
		svc := NewUserService(newFakeUserAdminRepository())
		if _, err := svc.AssignRoles(ctx, "nope", []string{"role_mgr"}); !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("err = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("the last admin cannot lose the admin role", func(t *testing.T) {
		svc := NewUserService(newFakeUserAdminRepository())
		a, _ := svc.CreateUser(ctx, domain.SaveUserInput{Name: "Andi", Email: "andi@company.co", RoleID: "role_master"})
		if _, err := svc.AssignRoles(ctx, a.ID, []string{"role_sa"}); !errors.Is(err, ErrLastAdmin) {
			t.Fatalf("err = %v, want ErrLastAdmin", err)
		}
	})
}
