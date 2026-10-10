ALTER TABLE peers
    ADD COLUMN nat_ip   inet,
    ADD COLUMN nat_port integer CHECK (nat_port > 0 AND nat_port < 65536);
