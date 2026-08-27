ALTER TABLE peers
    ADD COLUMN public_ip           inet,
    ADD COLUMN udp_port            integer CHECK (udp_port > 0 AND udp_port < 65536),
    ADD COLUMN endpoint_updated_at timestamptz,
    ADD COLUMN direct_reachable    boolean NOT NULL DEFAULT false;
