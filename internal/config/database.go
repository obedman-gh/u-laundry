package config

import (
	"net"
	"net/url"
	"strings"
	"time"
)

func (p *parser) database(production bool) Database {
	d := Database{
		Host: p.text("DB_HOST", "127.0.0.1"), Port: p.text("DB_PORT", "5432"),
		User: p.text("DB_USER", "ulaundry"), Password: p.get("DB_PASS"), Name: p.text("DB_NAME", "ulaundry"),
		SSLMode: p.text("DB_SSLMODE", "disable"), SSLRootCert: p.text("DB_SSLROOTCERT", ""),
		MaxOpenConns: p.integer("DB_MAX_OPEN_CONNS", 10, 1), MaxIdleConns: p.integer("DB_MAX_IDLE_CONNS", 5, 0),
		ConnMaxLifetime: p.duration("DB_CONN_MAX_LIFETIME", 30*time.Minute),
		ConnectTimeout:  p.duration("DB_CONNECT_TIMEOUT", 5*time.Second), PingTimeout: p.duration("DB_PING_TIMEOUT", 5*time.Second),
	}
	if raw := p.get("DB_DSN"); raw != "" {
		for _, key := range []string{"DB_HOST", "DB_PORT", "DB_USER", "DB_PASS", "DB_NAME", "DB_SSLMODE", "DB_SSLROOTCERT"} {
			if p.get(key) != "" {
				p.fail("DB_DSN", "tidak boleh digabung dengan komponen koneksi DB_*")
				break
			}
		}
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Opaque != "" || u.Hostname() == "" || u.User == nil || u.Fragment != "" {
			p.fail("DB_DSN", "harus URL postgres:// atau postgresql:// dengan host, user dan database")
		} else {
			d.Host, d.User, d.Name = u.Hostname(), u.User.Username(), strings.TrimPrefix(u.Path, "/")
			d.Password, _ = u.User.Password()
			d.Port = "5432"
			if u.Port() != "" {
				d.Port = u.Port()
			}
			if strings.HasSuffix(u.Host, ":") {
				p.fail("DB_DSN", "port tidak valid")
			}
			q, err := url.ParseQuery(u.RawQuery)
			if err != nil {
				p.fail("DB_DSN", "query URL tidak valid")
			}
			for key, values := range q {
				if (key != "sslmode" && key != "sslrootcert") || len(values) != 1 || values[0] == "" {
					p.fail("DB_DSN", "hanya satu sslmode dan sslrootcert yang diizinkan pada query URL")
				}
			}
			d.SSLMode, d.SSLRootCert = "disable", q.Get("sslrootcert")
			if q.Get("sslmode") != "" {
				d.SSLMode = q.Get("sslmode")
			}
		}
	} else if production {
		for _, key := range []string{"DB_HOST", "DB_USER", "DB_NAME", "DB_SSLMODE"} {
			if p.get(key) == "" {
				p.fail(key, "wajib diisi pada production")
			}
		}
	}
	if d.Host == "" || strings.ContainsAny(d.Host, " /\\,@?#\t\r\n") || (strings.Contains(d.Host, ":") && net.ParseIP(d.Host) == nil) {
		p.fail("DB_HOST/DB_DSN", "wajib satu hostname atau alamat IP TCP yang valid")
	}
	if !validPort(d.Port) {
		p.fail("DB_PORT/DB_DSN", "harus port 1–65535")
	}
	if strings.TrimSpace(d.User) == "" || strings.TrimSpace(d.Name) == "" || strings.ContainsRune(d.User+d.Name+d.Password, 0) {
		p.fail("DB_USER/DB_NAME/DB_DSN", "user dan database wajib diisi tanpa karakter NUL")
	}
	switch d.SSLMode {
	case "disable", "require", "verify-ca", "verify-full":
	default:
		p.fail("DB_SSLMODE/DB_DSN", "harus disable, require, verify-ca, atau verify-full")
	}
	if d.SSLRootCert == "system" && d.SSLMode != "verify-full" {
		p.fail("DB_SSLROOTCERT", "CA system wajib memakai verify-full")
	}
	if production {
		p.secret("DB_PASS/DB_DSN", d.Password, 16)
		if d.SSLMode != "verify-full" || d.SSLRootCert == "" {
			p.fail("DB_SSLMODE/DB_SSLROOTCERT", "production wajib verify-full dengan CA eksplisit")
		}
	}
	if d.MaxIdleConns > d.MaxOpenConns {
		p.fail("DB_MAX_IDLE_CONNS", "tidak boleh melebihi DB_MAX_OPEN_CONNS")
	}
	if d.ConnectTimeout > d.PingTimeout {
		p.fail("DB_CONNECT_TIMEOUT", "tidak boleh melebihi DB_PING_TIMEOUT")
	}
	return d
}
