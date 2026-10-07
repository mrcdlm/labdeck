-- Rohwerte von Kennzahlen einzelner Ressourcen (VMs, Container) werden
-- kürzer aufbewahrt – bei vielen Gästen wären es sonst Millionen Zeilen.
ALTER TABLE metric_samples ADD COLUMN short INTEGER NOT NULL DEFAULT 0;
