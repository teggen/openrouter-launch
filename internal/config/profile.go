package config

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrProfileExists is returned when a profile name is already taken.
	ErrProfileExists = errors.New("profile already exists")
	// ErrProfileNotFound is returned when no profile has the given name.
	ErrProfileNotFound = errors.New("profile not found")
	// ErrProfileInvalid is returned when required profile fields are empty.
	ErrProfileInvalid = errors.New("invalid profile")
)

// profileIndex returns the position of name, or -1. Names are compared
// case-insensitively so "Opus-CC" and "opus-cc" cannot both exist.
func (c *Config) profileIndex(name string) int {
	target := strings.ToLower(strings.TrimSpace(name))
	for i, p := range c.Profiles {
		if strings.ToLower(p.Name) == target {
			return i
		}
	}
	return -1
}

// Profile returns the named profile.
func (c *Config) Profile(name string) (Profile, bool) {
	i := c.profileIndex(name)
	if i < 0 {
		return Profile{}, false
	}
	return c.Profiles[i], true
}

// AddProfile appends a profile, rejecting duplicates and incomplete entries.
func (c *Config) AddProfile(p Profile) error {
	p.Name = strings.TrimSpace(p.Name)
	switch {
	case p.Name == "":
		return fmt.Errorf("%w: name is required", ErrProfileInvalid)
	case p.Agent == "":
		return fmt.Errorf("%w: agent is required", ErrProfileInvalid)
	case p.Model == "":
		return fmt.Errorf("%w: model is required", ErrProfileInvalid)
	}
	if c.profileIndex(p.Name) >= 0 {
		return fmt.Errorf("%w: %s", ErrProfileExists, p.Name)
	}
	c.Profiles = append(c.Profiles, p)
	return nil
}

// RemoveProfile deletes the named profile, preserving the order of the rest.
func (c *Config) RemoveProfile(name string) error {
	i := c.profileIndex(name)
	if i < 0 {
		return fmt.Errorf("%w: %s", ErrProfileNotFound, name)
	}
	c.Profiles = append(c.Profiles[:i], c.Profiles[i+1:]...)
	return nil
}

// RenameProfile changes a profile's name in place.
func (c *Config) RenameProfile(oldName, newName string) error {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return fmt.Errorf("%w: new name is required", ErrProfileInvalid)
	}
	i := c.profileIndex(oldName)
	if i < 0 {
		return fmt.Errorf("%w: %s", ErrProfileNotFound, oldName)
	}
	if j := c.profileIndex(newName); j >= 0 && j != i {
		return fmt.Errorf("%w: %s", ErrProfileExists, newName)
	}
	c.Profiles[i].Name = newName
	return nil
}
