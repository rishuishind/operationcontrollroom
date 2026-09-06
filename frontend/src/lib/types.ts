export interface WeatherConditions {
  TemperatureC: number;
  PrecipitationMM: number;
  PrecipProbability: number;
  WindSpeedKMH: number;
  WeatherCode: number;
}

export interface AreaAssessment {
  AreaID: number;
  AreaName: string;
  AvgTrips: number;
  SampleDays: number;
  WeatherOK: boolean;
  Multiplier: number;
  Adjusted: number;
  ExcessRatio: number;
  Weather: WeatherConditions;
}

export interface Recommendation {
  GeneratedAt: string;
  DayOfWeek: number;
  Hour: number;
  ActionNeeded: boolean;
  Target: AreaAssessment | null;
  Assessments: AreaAssessment[];
  Threshold: number;
  WeatherDegraded: boolean;
  Caveats: string[];
}
